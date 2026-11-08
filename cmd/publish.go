package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/gitexec"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/publish"
	"github.com/hivecommons/spektacular/internal/publish/github"
	"github.com/hivecommons/spektacular/internal/publish/jira"
	"github.com/hivecommons/spektacular/internal/store"
)

const (
	publishFormatPretty = "pretty"
	publishFormatJSON   = "json"
)

var publishCmd = &cobra.Command{
	Use:   "publish <name>",
	Short: "File a final spec or epic as an RFC issue with child work items in a work source",
	Long: `Publish turns a final spec (with its plan's tasks, or its requirements, as the
children) or a final epic (with its specs as the children) into one RFC item
plus child work items in a target work source, so an agent hive can pick the
work up. The default target is github: the RFC becomes an issue, each child a
sub-issue of it, and each depends_on edge a "blocked by" relation.

Every filed item carries a hidden marker, so a re-run finds what it filed and
updates it in place with --update, and refuses with a diff without it. The
publication is recorded in the artifact's frontmatter under published.

--target jira files the RFC as an Epic in publish.jira.project_key and each
child as a Story under it, with "Blocks" links for dependencies, on Jira Cloud
or Data Center.

--dry-run renders what would be filed and makes no network calls. Credentials
come from the environment: GITHUB_TOKEN, GH_TOKEN, or ` + "`gh auth token`" + ` for
github; JIRA_BASE_URL with JIRA_EMAIL and JIRA_API_TOKEN (Cloud) or JIRA_PAT
(Data Center) for jira. Nothing is written to config.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPublish,
}

// publishResult is the verb's JSON result.
type publishResult struct {
	publish.Result
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	ArtifactID string `json:"artifact_id"`
	DryRun     bool   `json:"dry_run"`
	// Publication is the full model, in --dry-run output only.
	Publication *publish.Publication `json:"publication,omitempty"`
}

var publishOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name":        {Type: "string"},
		"kind":        {Type: "string", Enum: []string{publish.KindSpec, publish.KindEpic}},
		"artifact_id": {Type: "string", Description: "<project>/<kind>/<name>; the marker's artifact_id"},
		"target":      {Type: "string", Enum: []string{config.PublishTargetGitHub, config.PublishTargetJira}},
		"repo":        {Type: "string", Description: "where the items were filed: owner/repo on github, the project key on jira"},
		"dry_run":     {Type: "boolean"},
		"rfc_url":     {Type: "string"},
		"rfc_number":  {Type: "integer", Description: "github only"},
		"rfc_key":     {Type: "string", Description: "the RFC's issue key on jira"},
		"children": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"id": {Type: "string"}, "url": {Type: "string"}, "number": {Type: "integer", Description: "github only"}, "key": {Type: "string", Description: "the issue key on jira"},
		}}},
		"created":   {Type: "array", Items: &schemaProp{Type: "string"}, Description: "rfc and child ids filed by this run"},
		"updated":   {Type: "array", Items: &schemaProp{Type: "string"}},
		"unchanged": {Type: "array", Items: &schemaProp{Type: "string"}},
		"warnings":  {Type: "array", Items: &schemaProp{Type: "string"}, Description: "best-effort steps that failed: labels not applied, relations not linked"},
		"publication": {Type: "object", Description: "the full model, --dry-run only", Properties: map[string]*schemaProp{
			"rfc": {Type: "object", Properties: map[string]*schemaProp{"title": {Type: "string"}, "body": {Type: "string"}, "marker": {Type: "string"}}},
			"children": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
				"id": {Type: "string"}, "title": {Type: "string"}, "body": {Type: "string"}, "repo": {Type: "string"},
				"depends_on": {Type: "array", Items: &schemaProp{Type: "string"}}, "marker": {Type: "string"},
			}}},
		}},
	},
	Required: []string{"name", "kind", "artifact_id", "target", "dry_run", "children"},
}

func runPublish(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: publishOutputSchema,
			Flags: map[string]*schemaProp{
				"target":      {Type: "string", Enum: []string{config.PublishTargetGitHub, config.PublishTargetJira, config.PublishTargetLinear}, Description: "work source; publish.default_target in config, else github. linear refuses publish_target_unconfigured until implemented"},
				"repo":        {Type: "string", Description: "github: owner/repo to file in; else publish.github.repo, else the project's source, else the cwd's origin"},
				"dry-run":     {Type: "boolean", Description: "render what would be filed; no network calls"},
				"update":      {Type: "boolean", Description: "update items already filed for this artifact in place instead of refusing"},
				"allow-draft": {Type: "boolean", Description: "publish an artifact that is not final, as RFC: [DRAFT] …"},
				"format":      {Type: "string", Enum: []string{publishFormatPretty, publishFormatJSON}},
			},
		}, "")
	}

	format, _ := cmd.Flags().GetString("format")
	if format != publishFormatPretty && format != publishFormatJSON {
		return output.NewError("publish_format_unsupported",
			fmt.Sprintf("publish format %q is not supported; supported formats: %s, %s", format, publishFormatPretty, publishFormatJSON)).
			WithResource(format).
			WithNextAction(fmt.Sprintf("re-run with --format %s or --format %s", publishFormatPretty, publishFormatJSON))
	}

	cfg, st, err := epicStore()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return output.NewError("publish_name_required", "a spec or epic name is required").
			WithNextAction(fmt.Sprintf("reissue naming the artifact; run `%[1]s epic list` or `%[1]s spec file list` to see what exists", cfg.Command))
	}
	name, err := publishName(cfg.Command, args)
	if err != nil {
		return err
	}

	targetName, _ := cmd.Flags().GetString("target")
	if targetName == "" {
		targetName = cfg.Publish.Target()
	}
	if targetName != config.PublishTargetGitHub && targetName != config.PublishTargetJira {
		return publishTargetUnconfigured(targetName)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	update, _ := cmd.Flags().GetBool("update")
	allowDraft, _ := cmd.Flags().GetBool("allow-draft")

	src, err := publishSource(cfg, st, name, targetName, allowDraft)
	if err != nil {
		return err
	}
	pub, err := publish.Build(src)
	if err != nil {
		return err
	}
	if err := publish.Validate(pub); err != nil {
		return publishRefusal(err, name)
	}

	res := publishResult{Name: pub.Name, Kind: pub.Kind, ArtifactID: pub.ArtifactID, DryRun: dryRun}
	res.Target = targetName
	res.Children = []publish.ResultChild{}
	res.Created, res.Updated, res.Unchanged, res.Warnings = []string{}, []string{}, []string{}, []string{}

	if dryRun {
		for _, c := range pub.Children {
			res.Children = append(res.Children, publish.ResultChild{ID: c.ID})
		}
		res.Repo = publishDryRunWhere(cfg, cmd, targetName)
		res.Publication = &pub
		if format == publishFormatJSON {
			return output.New(cmd.OutOrStdout(), globalFields).WriteResult(res)
		}
		_, err := fmt.Fprint(cmd.OutOrStdout(), publish.RenderMarkdown(pub))
		return err
	}

	target, opts, where, err := publishTargetFor(cfg, cmd, targetName)
	if err != nil {
		return err
	}
	res.Repo = where

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	existing, err := target.Find(ctx, pub.ArtifactID)
	if err != nil {
		return output.NewError(publish.ErrTargetFailed, err.Error()).
			WithNextAction("check the credential's access to " + where + " and retry")
	}
	if existing != nil {
		if existing.RFC != nil && existing.RFC.Closed {
			return output.NewError(publish.ErrConflict,
				fmt.Sprintf("%s %q was already published to %s as %s, which is closed", pub.Kind, name, where, existing.RFC.URL)).
				WithResource(existing.RFC.URL).
				WithNextAction("reopen the RFC and re-run with --update, or change the artifact's name to publish it afresh")
		}
		changes := publish.Diff(pub, existing)
		if len(changes) > 0 && !update {
			at := "children only"
			if existing.RFC != nil {
				at = existing.RFC.URL
			}
			return output.NewError(publish.ErrConflict,
				fmt.Sprintf("%s %q is already published to %s (%s) and re-publishing would change: %s", pub.Kind, name, where, at, publish.DescribeChanges(changes))).
				WithResource(at).
				WithNextAction("re-run with --update to apply the changes in place, or --dry-run to see what would be filed")
		}
	}

	result, err := target.Publish(ctx, pub, existing, opts)
	res.Result = result
	res.Target = targetName
	if err != nil {
		return output.NewError(publish.ErrTargetFailed, fmt.Sprintf("publishing %s %q to %s failed part way: %v", pub.Kind, name, where, err)).
			WithNextAction("fix the cause and re-run with --update; items already filed are found by their markers and reused")
	}

	if err := writePublished(cfg, st, pub, result); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the publication was filed but could not be recorded in the %s's frontmatter: %v", pub.Kind, err))
	}

	if format == publishFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(res)
	}
	return renderPublishPretty(cmd, res)
}

func renderPublishPretty(cmd *cobra.Command, res publishResult) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Published %s %s to %s\n", res.Kind, res.Name, res.Repo)
	fmt.Fprintf(w, "  RFC: %s\n", res.RFCURL)
	for _, c := range res.Children {
		fmt.Fprintf(w, "  %s: %s\n", c.ID, c.URL)
	}
	if len(res.Created) > 0 {
		fmt.Fprintf(w, "created: %s\n", strings.Join(res.Created, ", "))
	}
	if len(res.Updated) > 0 {
		fmt.Fprintf(w, "updated: %s\n", strings.Join(res.Updated, ", "))
	}
	if len(res.Unchanged) > 0 {
		fmt.Fprintf(w, "unchanged: %s\n", strings.Join(res.Unchanged, ", "))
	}
	for _, warning := range res.Warnings {
		fmt.Fprintf(w, "warning: %s\n", warning)
	}
	return nil
}

func publishTargetUnconfigured(name string) error {
	switch name {
	case config.PublishTargetLinear:
		return output.NewError(publish.ErrTargetUnconfigured,
			fmt.Sprintf("the %s target is not available yet in this release", name)).
			WithResource(name).
			WithNextAction("re-run with --target github or --target jira, or set publish.default_target in .spektacular/config.yaml")
	}
	return output.NewError(publish.ErrTargetUnconfigured,
		fmt.Sprintf("unknown publish target %q; supported targets: %s, %s", name, config.PublishTargetGitHub, config.PublishTargetJira)).
		WithResource(name).
		WithNextAction("re-run with --target github or --target jira")
}

// publishTargetFor builds the target named, its options, and where it
// files (owner/repo, a project key), refusing when the target lacks a
// destination or a credential.
func publishTargetFor(cfg config.Config, cmd *cobra.Command, name string) (publish.Target, publish.PublishOptions, string, error) {
	switch name {
	case config.PublishTargetJira:
		t, err := jiraTarget(cfg.Publish.Jira)
		if err != nil {
			return nil, publish.PublishOptions{}, "", err
		}
		return t, publish.PublishOptions{Labels: cfg.Publish.Jira.Labels}, t.Slug(), nil
	default:
		owner, repoName, ok := publishRepo(cfg, cmd)
		if !ok {
			return nil, publish.PublishOptions{}, "", output.NewError(publish.ErrRepoUnknown,
				"no GitHub repository to publish into: none was given with --repo, publish.github.repo is not set, and neither the project's source nor the current directory's origin is a GitHub URL").
				WithNextAction("re-run with --repo <owner>/<repo>, or set publish.github.repo in .spektacular/config.yaml")
		}
		token, err := githubToken()
		if err != nil {
			return nil, publish.PublishOptions{}, "", err
		}
		t := github.New(owner, repoName, token)
		if api := os.Getenv("SPEKTACULAR_GITHUB_API"); api != "" {
			t.API = api
		}
		opts := publish.PublishOptions{ParentLabel: cfg.Publish.GitHub.ParentLabelName(), Labels: cfg.Publish.GitHub.Labels}
		return t, opts, t.Slug(), nil
	}
}

// publishDryRunWhere is the destination a dry run reports, without
// touching credentials: it may be empty when none is configured.
func publishDryRunWhere(cfg config.Config, cmd *cobra.Command, target string) string {
	if target == config.PublishTargetJira {
		return strings.TrimSpace(cfg.Publish.Jira.ProjectKey)
	}
	if repo, _ := cmd.Flags().GetString("repo"); repo != "" {
		return repo
	}
	if owner, r, ok := publishRepo(cfg, cmd); ok {
		return owner + "/" + r
	}
	return ""
}

// Environment the Jira target reads. The site and credentials never come
// from config; JIRA_BASE_URL falls back to publish.jira.base_url.
const (
	envJiraBaseURL  = "JIRA_BASE_URL"
	envJiraEmail    = "JIRA_EMAIL"
	envJiraAPIToken = "JIRA_API_TOKEN"
	envJiraPAT      = "JIRA_PAT"
	envJiraCABundle = "JIRA_CA_BUNDLE"
)

// jiraTarget builds the Jira target from config and the environment. Each
// refusal names the missing setting.
func jiraTarget(jc config.PublishJiraConfig) (*jira.Target, error) {
	projectKey := strings.TrimSpace(jc.ProjectKey)
	if projectKey == "" {
		return nil, output.NewError(publish.ErrTargetUnconfigured,
			"the jira target has no project to file into: publish.jira.project_key is not set").
			WithResource(config.PublishTargetJira).
			WithNextAction("set publish.jira.project_key in .spektacular/config.yaml to the key of the project the hive reads")
	}
	baseURL := strings.TrimSpace(os.Getenv(envJiraBaseURL))
	if baseURL == "" {
		baseURL = strings.TrimSpace(jc.BaseURL)
	}
	if baseURL == "" {
		return nil, output.NewError(publish.ErrTargetUnconfigured,
			fmt.Sprintf("the jira target has no site to file into: %s is unset and publish.jira.base_url is not set", envJiraBaseURL)).
			WithResource(config.PublishTargetJira).
			WithNextAction(fmt.Sprintf("export %s=https://<site>.atlassian.net (or your Data Center URL), or set publish.jira.base_url", envJiraBaseURL))
	}
	t := jira.New(baseURL, projectKey)
	t.DataCenter = jc.IsDataCenter()
	t.ParentIssueType = jc.ParentIssueTypeName()
	t.ChildIssueType = jc.ChildIssueTypeName()
	t.EpicLinkField = strings.TrimSpace(jc.EpicLinkField)
	t.AcceptanceCriteriaField = strings.TrimSpace(jc.AcceptanceCriteriaField)
	t.PAT = strings.TrimSpace(os.Getenv(envJiraPAT))
	t.Email = strings.TrimSpace(os.Getenv(envJiraEmail))
	t.APIToken = strings.TrimSpace(os.Getenv(envJiraAPIToken))
	switch {
	case t.DataCenter && t.PAT == "" && (t.Email == "" || t.APIToken == ""):
		return nil, output.NewError(publish.ErrAuthMissing,
			fmt.Sprintf("no Jira Data Center credential: %s is unset (and %s + %s are not both set)", envJiraPAT, envJiraEmail, envJiraAPIToken)).
			WithNextAction(fmt.Sprintf("export %s with a personal access token that can create issues in %s", envJiraPAT, projectKey))
	case !t.DataCenter && t.PAT == "" && (t.Email == "" || t.APIToken == ""):
		return nil, output.NewError(publish.ErrAuthMissing,
			fmt.Sprintf("no Jira Cloud credential: %s and %s must both be set", envJiraEmail, envJiraAPIToken)).
			WithNextAction(fmt.Sprintf("export %s and %s (an Atlassian API token) for an account that can create issues in %s", envJiraEmail, envJiraAPIToken, projectKey))
	}
	client, err := jira.NewHTTPClient(strings.TrimSpace(os.Getenv(envJiraCABundle)))
	if err != nil {
		return nil, output.NewError(publish.ErrTargetUnconfigured, fmt.Sprintf("%s: %v", envJiraCABundle, err)).
			WithResource(config.PublishTargetJira).
			WithNextAction(fmt.Sprintf("point %s at a PEM bundle, or unset it to use the system roots", envJiraCABundle))
	}
	t.HTTPClient = client
	return t, nil
}

// publishName parses the single <name> argument as a spec or epic name.
func publishName(command string, args []string) (string, error) {
	addr, err := artifact.Parse(artifact.KindSpec, args)
	if err != nil {
		var extErr *artifact.ExtensionError
		if errors.As(err, &extErr) {
			return "", output.NewError(artifact.ErrCodeUnexpectedExtension,
				fmt.Sprintf("%q carries a file extension or path; artifacts are addressed by bare name", args[0])).
				WithResource(args[0]).
				WithNextAction(fmt.Sprintf("reissue with the bare name %q", extErr.Corrected.Feature))
		}
		return "", output.NewError("bad_input", fmt.Sprintf("invalid artifact name %q: %v", args[0], err)).
			WithResource(args[0]).
			WithNextAction(fmt.Sprintf("run `%[1]s epic list` or `%[1]s spec file list` to see the names it accepts", command))
	}
	return addr.Feature, nil
}

// publishRefusal maps a publish.Error to the JSON envelope.
func publishRefusal(err error, name string) error {
	var perr *publish.Error
	if errors.As(err, &perr) {
		return output.NewError(perr.Code, perr.Message).WithResource(name).WithNextAction(perr.NextAction)
	}
	return err
}

// publishSource reads the named artifact and everything the model is built
// from. An epic name publishes the epic with its specs as children; a spec
// name publishes that spec alone, with its plan's tasks as children when it
// has a task-format plan, else its requirements; a plan name resolves to its
// spec.
func publishSource(cfg config.Config, st store.Store, name, target string, allowDraft bool) (publish.Source, error) {
	src := publish.Source{Target: target, DefaultRepo: singleRepo(cfg), Provenance: publishProvenance(cfg, st)}

	if raw, err := st.Read(epicPath(cfg, name)); err == nil {
		e, err := epic.Parse(raw)
		if err != nil {
			return src, output.NewError("epic_malformed", fmt.Sprintf("epic %q cannot be read: %v", name, err)).
				WithResource(name).
				WithNextAction("repair the epic's frontmatter, then re-run publish")
		}
		if err := publishFinal(cfg, publish.KindEpic, name, e.DocumentStatus, allowDraft); err != nil {
			return src, err
		}
		src.Kind, src.Name = publish.KindEpic, name
		src.ArtifactID = publishArtifactID(cfg, publish.KindEpic, name)
		src.Draft = e.DocumentStatus != metadata.StatusFinal
		src.Body = e.Body
		for _, member := range e.Specs {
			m := publish.Member{Name: member.Name, DependsOn: append([]string{}, member.DependsOn...)}
			if specRaw, err := st.Read(specStorePath(cfg, member.Name)); err == nil {
				if _, body, err := metadata.Split(specRaw); err == nil {
					m.Body = body
				} else {
					m.Body = specRaw
				}
			}
			src.Members = append(src.Members, m)
		}
		return src, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return src, err
	}

	specName := name
	raw, err := st.Read(specStorePath(cfg, specName))
	if errors.Is(err, store.ErrNotFound) {
		// A plan name resolves to its spec.
		planRaw, planErr := st.Read(planStorePath(cfg, name))
		if planErr != nil {
			if errors.Is(planErr, store.ErrNotFound) {
				return src, output.NewError("artifact_not_found",
					fmt.Sprintf("no epic, spec or plan named %q was found (searched the epic store %q, the spec store %q and the plan store %q)",
						name, cfg.Epic.Config.Directory, cfg.Spec.Config.Directory, cfg.Plan.Config.Directory)).
					WithResource(name).
					WithNextAction(fmt.Sprintf("run `%[1]s epic list`, `%[1]s spec file list` or `%[1]s plan file list` to see what exists", cfg.Command))
			}
			return src, planErr
		}
		if fm, _, splitErr := metadata.Split(planRaw); splitErr == nil && fm != nil && fm.Spec != "" {
			specName = fm.Spec
		}
		raw, err = st.Read(specStorePath(cfg, specName))
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return src, output.NewError("artifact_not_found", fmt.Sprintf("spec %q was not found", specName)).
				WithResource(specName).
				WithNextAction(fmt.Sprintf("run `%s spec file list` to see the stored specs", cfg.Command))
		}
		return src, err
	}
	fm, body, err := metadata.Split(raw)
	if err != nil {
		return src, output.NewError("metadata_read_failed", fmt.Sprintf("could not read spec %q: %v", specName, err)).
			WithResource(specName).
			WithNextAction("repair the spec's frontmatter, then re-run publish")
	}
	var status metadata.DocumentStatus
	if fm != nil {
		status = fm.DocumentStatus
	}
	if err := publishFinal(cfg, publish.KindSpec, specName, status, allowDraft); err != nil {
		return src, err
	}
	src.Kind, src.Name = publish.KindSpec, specName
	src.ArtifactID = publishArtifactID(cfg, publish.KindSpec, specName)
	src.Draft = status != metadata.StatusFinal
	src.Body = body
	if planRaw, err := st.Read(planStorePath(cfg, specName)); err == nil {
		_, planBody, splitErr := metadata.Split(planRaw)
		if splitErr != nil {
			planBody = planRaw
		}
		plan := plantask.Parse(planBody)
		if plan.Format == plantask.FormatTasks {
			src.Plan = &plan
			src.PlanName = specName
		}
	}
	return src, nil
}

// publishFinal refuses an artifact that is not final unless --allow-draft.
func publishFinal(cfg config.Config, kind, name string, status metadata.DocumentStatus, allowDraft bool) error {
	if status == metadata.StatusFinal || allowDraft {
		return nil
	}
	shown := string(status)
	if shown == "" {
		shown = "unset"
	}
	next := fmt.Sprintf("finish the %s so its document_status is final, or re-run with --allow-draft to publish it as RFC: [DRAFT]", kind)
	if kind == publish.KindSpec {
		next = fmt.Sprintf("finish the spec with `%s spec finish %s`, or re-run with --allow-draft to publish it as RFC: [DRAFT]", cfg.Command, name)
	}
	return output.NewError(publish.ErrNotFinal,
		fmt.Sprintf("%s %q has document_status %s; only a final %s is published", kind, name, shown, kind)).
		WithResource(name).
		WithNextAction(next)
}

// publishArtifactID is the marker's artifact_id: <project>/<kind>/<name>,
// unique across the projects that publish into one repository.
func publishArtifactID(cfg config.Config, kind, name string) string {
	project := cfg.Name
	if project == "" {
		project = "project"
	}
	return project + "/" + kind + "/" + name
}

// singleRepo is the project's one registered repo, or empty.
func singleRepo(cfg config.Config) string {
	if len(cfg.Repos) == 1 {
		return cfg.Repos[0].Name
	}
	return ""
}

func publishProvenance(cfg config.Config, st store.Store) publish.Provenance {
	p := publish.Provenance{Project: cfg.Name, Repo: cfg.Source, Version: versionString()}
	if sha, err := gitexec.Run(st.Root(), nil, "rev-parse", "--short", "HEAD"); err == nil {
		p.Commit = strings.TrimSpace(sha)
	}
	return p
}

func specStorePath(cfg config.Config, name string) string {
	return artifact.Address{Kind: artifact.KindSpec, Feature: name}.StorePath(cfg.Spec.Config.Directory)
}

func planStorePath(cfg config.Config, name string) string {
	return artifact.Address{Kind: artifact.KindPlan, Feature: name, Document: "plan"}.StorePath(cfg.Plan.Config.Directory)
}

// publishRepo resolves the GitHub repository: --repo, then
// publish.github.repo, then the project's source, then the cwd's origin.
func publishRepo(cfg config.Config, cmd *cobra.Command) (owner, repo string, ok bool) {
	if flag, _ := cmd.Flags().GetString("repo"); flag != "" {
		return parseGitHubRepo(flag)
	}
	if cfg.Publish.GitHub.Repo != "" {
		return parseGitHubRepo(cfg.Publish.GitHub.Repo)
	}
	if cfg.Source != "" {
		if o, r, ok := parseGitHubRepo(cfg.Source); ok {
			return o, r, true
		}
	}
	if out, err := gitexec.Run(".", nil, "remote", "get-url", "origin"); err == nil {
		return parseGitHubRepo(strings.TrimSpace(out))
	}
	return "", "", false
}

var (
	githubSlugRE = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?$`)
	githubURLRE  = regexp.MustCompile(`^(?:https?://|git@|ssh://git@|github:)?(?:www\.)?github\.com[/:]([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?/?$`)
)

// parseGitHubRepo reads owner/repo from a slug or a GitHub https/ssh URL.
func parseGitHubRepo(s string) (owner, repo string, ok bool) {
	s = strings.TrimSpace(s)
	if m := githubURLRE.FindStringSubmatch(s); m != nil {
		return m[1], m[2], true
	}
	if m := githubSlugRE.FindStringSubmatch(s); m != nil && !strings.Contains(s, "://") {
		return m[1], m[2], true
	}
	return "", "", false
}

// githubToken is GITHUB_TOKEN, GH_TOKEN, or what `gh auth token` prints.
// Nothing is written to config.
func githubToken() (string, error) {
	for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, nil
		}
	}
	if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
		if v := strings.TrimSpace(string(out)); v != "" {
			return v, nil
		}
	}
	return "", output.NewError(publish.ErrAuthMissing,
		"no GitHub credential: GITHUB_TOKEN and GH_TOKEN are unset and `gh auth token` printed nothing").
		WithNextAction("export GITHUB_TOKEN with a token that can write issues in the repository, or run `gh auth login`")
}

// writePublished records the result in the artifact's frontmatter, replacing
// any earlier entry for the same target.
func writePublished(cfg config.Config, st store.Store, pub publish.Publication, result publish.Result) error {
	entry := metadata.Publication{Target: result.Target, RFCURL: result.RFCURL, Children: []metadata.PublishedChild{}}
	for _, c := range result.Children {
		entry.Children = append(entry.Children, metadata.PublishedChild{ID: c.ID, URL: c.URL})
	}
	switch pub.Kind {
	case publish.KindEpic:
		path := epicPath(cfg, pub.Name)
		raw, err := st.Read(path)
		if err != nil {
			return err
		}
		e, err := epic.Parse(raw)
		if err != nil {
			return err
		}
		e.Published = replacePublication(e.Published, entry)
		out, err := e.Render()
		if err != nil {
			return err
		}
		return st.Write(path, out)
	default:
		path := specStorePath(cfg, pub.Name)
		raw, err := st.Read(path)
		if err != nil {
			return err
		}
		fm, body, err := metadata.Split(raw)
		if err != nil {
			return err
		}
		if fm == nil {
			return fmt.Errorf("spec %q has no frontmatter to record the publication in", pub.Name)
		}
		fm.Published = replacePublication(fm.Published, entry)
		out, err := metadata.Render(*fm, body)
		if err != nil {
			return err
		}
		return st.Write(path, out)
	}
}

func replacePublication(list []metadata.Publication, entry metadata.Publication) []metadata.Publication {
	out := make([]metadata.Publication, 0, len(list)+1)
	for _, p := range list {
		if p.Target != entry.Target {
			out = append(out, p)
		}
	}
	return append(out, entry)
}

func init() {
	publishCmd.Flags().Bool("schema", false, "Print the input/output schema and exit")
	publishCmd.Flags().String("target", "", "Work source to publish into: github (default), jira, linear")
	publishCmd.Flags().String("repo", "", "GitHub owner/repo to file in; defaults to publish.github.repo, then the project's source, then the cwd's origin")
	publishCmd.Flags().Bool("dry-run", false, "Render what would be filed and make no network calls")
	publishCmd.Flags().Bool("update", false, "Update items already filed for this artifact in place")
	publishCmd.Flags().Bool("allow-draft", false, "Publish an artifact that is not final, titled RFC: [DRAFT] …")
	publishCmd.Flags().String("format", publishFormatPretty, "Output format: pretty or json")
}
