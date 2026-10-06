---
created_date: "2026-10-02"
document_status: final
closed_date: "2026-10-02"
---

# Test plan: 000060_epics-and-seeded-specs

The third success metric ("one command shows where an epic stands") is covered by automated
behavioural tests (`cmd/status_test.go`, `internal/status/status_test.go`) and the retired-command
tests, so it is not repeated here. The two metrics below depend on agent judgement and are checked
by hand with a real agent.

## Setup (both procedures)

Run against a released build installed on `PATH` (`make install-local`), never `go run .`, in a
throwaway project:

```
mkdir /tmp/spek-000060 && cd /tmp/spek-000060 && git init
spektacular init claude
```

Open Claude Code in that directory. Record each session's transcript (the agent's questions and the
CLI calls) for the result.

## 1. A seeded spec asks only about the sections the issue left out

- **What to measure:** for a spec started from a tracker issue that covers its overview,
  requirements and acceptance criteria, the number of interview or section-step questions that
  re-ask something the issue already answers. **Threshold: 0.**
- **How:**
  1. Create a GitHub issue (any repository the agent can read) whose body gives a clear
     description (overview), a bullet list of behaviours (requirements) and a task list of
     pass/fail checks (acceptance criteria), and says nothing about constraints, technical
     approach, success metrics or non-goals.
  2. Repeat three times, once per phrasing, each in a fresh throwaway project:
     `spec from #<n>`, `start a spec from <issue URL>`, `/spek:new` then "use issue <n>".
  3. Check that the agent fetched the issue with its own tools, that `spec new` was called with
     `"sources":[{"uri":"<issue URL>"}]`, and that the interview opened by listing the gaps
     (constraints, technical approach, success metrics, non-goals).
  4. Count every question in the interview and the overview, requirements and acceptance-criteria
     steps that asks for content the issue already gave, rather than presenting a seeded draft to
     confirm.
  5. After `finished`, run `spektacular spec file read <name>` and confirm the frontmatter has
     `sources` with the issue URL and today's `retrieved_date`.
- **Expected result:** 0 re-asked questions in each of the three runs; the gap list names exactly
  the four uncovered sections; each covered section is presented as a draft to confirm; `sources`
  is recorded. Additionally, with an unreachable source (a private repository the agent cannot
  read), the agent says it cannot reach it and asks for the content instead of starting a blank
  interview.
- **Who / when:** a maintainer, before tagging the release that ships 000060.

## 2. Code plus its docs, tests or config never receives a split offer

- **What to measure:** split offers made at the `split` step for single changes made of code plus
  supporting work. **Threshold: 0 offers across all scenarios.**
- **How:** run the spec workflow to `finished` once per scenario below, each in a fresh throwaway
  project, answering the interview with exactly the scenario's content:
  1. "Add a `--json` flag to `spec list`, document it on the docs site and add tests for it."
  2. "Add a `retry_limit` config key with a migration step, a default, validation and docs."
  3. "Rename the `plan file` verbs, update every skill and template that mentions them, and
     update the README."
  4. Run scenario 1 three times: with `epic_split_threshold: lenient`, `moderate` and `strict`
     set in `.spektacular/config.yaml`.

  As a control, run one scenario that should split: "Add epics to group specs, and separately add
  a web dashboard that renders the status JSON", with `epic_split_threshold: moderate`.
- **Expected result:** at the `split` step, scenarios 1–4 produce no offer at any threshold, and
  no `.spektacular/epics/` folder exists afterwards. The control produces an offer naming at least
  two specs with a one-line scope each, and declining it leaves no epic and no new specs.
- **Who / when:** a maintainer, before tagging the release that ships 000060.

## Harbor suites still to run

The task "Run the harbor suites" was marked done with only the spec-workflow suite run (passed,
job `2026-10-02__08-28-49`). Before release, also run:

```
make harbor-test-plan        # expects `status <name> --format json` to list specs[0].plan.tasks
make harbor-test-implement
```

- **Expected result:** both suites pass (reward 1.0).
- **Who / when:** a maintainer, before tagging the release that ships 000060.
