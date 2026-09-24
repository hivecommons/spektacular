package main

import (
	"context"
	"dagger/spektacular/internal/dagger"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/log"
)

var oses = []string{"linux", "darwin", "windows"}
var arches = []string{"amd64", "arm64"}
var owner = "hivecommons"
var repo = "spektacular"

// brewTaps are the Homebrew taps a release publishes the formula to, in order.
//
// hivecommons/homebrew-repo is the canonical tap. jumppad-labs/homebrew-repo is
// published to as well for the duration of the org migration, so anyone who
// already ran `brew tap jumppad-labs/homebrew-repo` keeps receiving updates
// rather than silently pinning to the last pre-move release. Drop the legacy
// entry once the deprecation window closes; the release token must hold write
// access to every tap listed here.
var brewTaps = []string{
	"hivecommons/homebrew-repo",
	"jumppad-labs/homebrew-repo",
}

func New() *Spektacular {
	return &Spektacular{}
}

type Spektacular struct {
	lastError     error
	goCacheVolume *dagger.CacheVolume
}

func (d *Spektacular) WithGoCache(cache *dagger.CacheVolume) *Spektacular {
	d.goCacheVolume = cache
	return d
}

func (d *Spektacular) All(
	ctx context.Context,
	src *dagger.Directory,
	// +optional
	quick bool,
	// +optional
	githubToken *dagger.Secret,
	// +optional
	notorizeCert *dagger.File,
	// +optional
	notorizeCertPassword *dagger.Secret,
	// +optional
	notorizeKey *dagger.File,
	// +optional
	notorizeId string,
	// +optional
	notorizeIssuer string,
) (*dagger.Directory, error) {
	// if quick, only build for the current architecture
	if quick {
		d.setArchLocalMachine(ctx)
	}

	// get the version
	version := "0.0.0"
	sha := ""

	var output *dagger.Directory

	// remove the build output directory from the source
	src = src.
		WithoutDirectory(".dagger").
		WithoutDirectory("build-output")

	// if we have a github token, get the version from the associated PR label
	if githubToken != nil {
		version, sha, _ = d.getVersion(ctx, githubToken, src)
	}

	log.Info("Building version", "semver", version, "sha", sha)

	// run the unit tests
	d.UnitTest(ctx, src, !quick)

	// build the applications
	output, _ = d.Build(ctx, src, version, sha)

	// package the build outputs
	output, _ = d.Package(ctx, output, version)

	// create the archives
	output, _ = d.Archive(ctx, output, version)

	// if we have the notorization details sign and notorize the osx binaries
	if notorizeCert != nil && notorizeCertPassword != nil && notorizeKey != nil && notorizeId != "" && notorizeIssuer != "" {
		output, _ = d.SignAndNotorize(ctx, version, output, notorizeCert, notorizeCertPassword, notorizeKey, notorizeId, notorizeIssuer)
	}

	// generate the checksums
	output, _ = d.GenerateChecksums(ctx, output, version)

	return output, d.lastError
}

func (d *Spektacular) Release(
	ctx context.Context,
	src *dagger.Directory,
	archives *dagger.Directory,
	githubToken *dagger.Secret,
	gemfuryToken *dagger.Secret,
) (string, error) {
	// create a new github release
	version, _ := d.GithubRelease(ctx, src, archives, githubToken)

	// update the brew formula in every tap listed in brewTaps
	d.UpdateBrew(ctx, version, githubToken)

	//	update the gemfury repository
	d.UpdateGemFury(ctx, version, gemfuryToken, archives)

	return version, d.lastError
}

func (d *Spektacular) Build(
	ctx context.Context,
	src *dagger.Directory,
	version,
	sha string,
) (*dagger.Directory, error) {
	if d.hasError() {
		return nil, d.lastError
	}

	cli := dag

	// create empty directory to put build outputs
	outputs := cli.Directory()

	// get `golang` image
	golang := cli.Container().
		From("golang:latest").
		WithDirectory("/src", src).
		WithWorkdir("/src").
		WithMountedCache("/go/pkg/mod", d.goCache())

	for _, goos := range oses {
		for _, goarch := range arches {
			fmt.Println("Build for", goos, goarch, "...")

			// create a directory for each os and arch
			path := fmt.Sprintf("build/%s/%s/", goos, goarch)

			// set GOARCH and GOOS in the build environment
			build, err := golang.
				WithEnvVariable("CGO_ENABLED", "0").
				WithEnvVariable("GOOS", goos).
				WithEnvVariable("GOARCH", goarch).
				WithExec([]string{
					"go", "build",
					"-o", path,
					"-ldflags", fmt.Sprintf("-X github.com/hivecommons/spektacular/cmd.version=%s -X github.com/hivecommons/spektacular/cmd.sha=%s", version, sha),
				}).
				Sync(ctx)

			if err != nil {
				d.lastError = err
				return nil, err
			}

			// get reference to build output directory in container
			outputs = outputs.WithDirectory(path, build.Directory(path))
		}
	}

	return outputs, nil
}

func (d *Spektacular) UnitTest(
	ctx context.Context,
	src *dagger.Directory,
	withRace bool,
) error {
	if d.hasError() {
		return d.lastError
	}

	cli := dag

	raceFlag := ""
	if withRace {
		raceFlag = "-race"
	}

	golang := cli.Container().
		From("golang:latest").
		WithDirectory("/src", src).
		WithMountedCache("/go/pkg/mod", d.goCache()).
		WithWorkdir("/src").
		WithExec([]string{"go", "test", "-v", raceFlag, "./..."})

	_, err := golang.Sync(ctx)
	if err != nil {
		d.lastError = err
	}

	return err
}

func (d *Spektacular) Package(
	ctx context.Context,
	binaries *dagger.Directory,
	version string,
) (*dagger.Directory, error) {
	if d.hasError() {
		return nil, d.lastError
	}

	cli := dag

	for _, os := range oses {
		if os == "linux" {
			for _, a := range arches {
				// create a package directory including the binaries
				pkg := cli.Directory()
				pkg = pkg.WithFile("/bin/spektacular", binaries.File(fmt.Sprintf("/build/linux/%s/spektacular", a)))

				// create a debian package
				p := cli.Deb().Build(pkg, a, "spektacular", version, "Nic Jackson", "Spektacular application")

				// add the debian package to the binaries directory
				binaries = binaries.WithFile(fmt.Sprintf("/pkg/linux/%s/spektacular.deb", a), p)
			}
		}
	}

	return binaries, nil
}

type Archive struct {
	// path of the
	Path   string
	Type   string
	Output string
}

var archives = []Archive{
	{Path: "/build/windows/amd64/spektacular.exe", Type: "zip", Output: "spektacular_%%VERSION%%_windows_x86_64.zip"},
	{Path: "/build/darwin/amd64/spektacular", Type: "zip", Output: "spektacular_%%VERSION%%_darwin_x86_64.zip"},
	{Path: "/build/darwin/arm64/spektacular", Type: "zip", Output: "spektacular_%%VERSION%%_darwin_arm64.zip"},
	{Path: "/build/linux/amd64/spektacular", Type: "targz", Output: "spektacular_%%VERSION%%_linux_x86_64.tar.gz"},
	{Path: "/build/linux/arm64/spektacular", Type: "targz", Output: "spektacular_%%VERSION%%_linux_arm64.tar.gz"},
	{Path: "/pkg/linux/amd64/spektacular.deb", Type: "copy", Output: "spektacular_%%VERSION%%_linux_x86_64.deb"},
	{Path: "/pkg/linux/arm64/spektacular.deb", Type: "copy", Output: "spektacular_%%VERSION%%_linux_arm64.deb"},
}

// Archive creates zipped and tar archives of the binaries
func (d *Spektacular) Archive(
	ctx context.Context,
	binaries *dagger.Directory,
	version string,
) (*dagger.Directory, error) {
	if d.hasError() {
		return nil, d.lastError
	}

	cli := dag
	out := cli.Directory()

	zipContainer := cli.Container().
		From("alpine:latest").
		WithExec([]string{"apk", "add", "zip"})

	checksums := strings.Builder{}

	for _, a := range archives {
		outPath := strings.ReplaceAll(a.Output, "%%VERSION%%", version)
		switch a.Type {
		case "zip":
			// create a zip archive

			// first get the filename as with windows this has an extension
			fn := path.Base(a.Path)

			// zip the file
			zip := zipContainer.
				WithMountedFile(fn, binaries.File(a.Path)).
				WithExec([]string{"zip", "-r", outPath, fn})

			out = out.WithFile(outPath, zip.File(outPath))
		case "targz":
			// create a zip archive
			zip := zipContainer.
				WithMountedFile("/spektacular", binaries.File(a.Path)).
				WithExec([]string{"tar", "-czf", outPath, "/spektacular"})

			out = out.WithFile(outPath, zip.File(outPath))
		case "copy":
			out = out.WithFile(outPath, binaries.File(a.Path))
		}

		// generate the checksum
		cs, err := cli.Checksum().CalculateFromFile(ctx, out.File(outPath))
		if err != nil {
			d.lastError = fmt.Errorf("unable to generate checksum for archive: %w", err)
			return nil, d.lastError
		}

		// checksum is returned as "checksum filename" we need to remove the filename as it is not
		// the same as the release name
		csParts := strings.Split(cs, " ")

		checksums.WriteString(fmt.Sprintf("%s  %s\n", csParts[0], outPath))
	}

	out = out.WithNewFile("checksums.txt", checksums.String())

	return out, nil
}

func (d *Spektacular) GenerateChecksums(
	ctx context.Context,
	files *dagger.Directory,
	version string,
) (*dagger.Directory, error) {
	if d.hasError() {
		return nil, d.lastError
	}

	cli := dag
	checksums := strings.Builder{}

	for _, a := range archives {
		outPath := strings.ReplaceAll(a.Output, "%%VERSION%%", version)

		// generate the checksum
		cs, err := cli.Checksum().CalculateFromFile(ctx, files.File(outPath))
		if err != nil {
			d.lastError = fmt.Errorf("unable to generate checksum for archive: %w", err)
			return nil, d.lastError
		}

		// checksum is returned as "checksum filename" we need to remove the filename as it is not
		// the same as the release name
		csParts := strings.Split(cs, " ")

		checksums.WriteString(fmt.Sprintf("%s  %s\n", csParts[0], outPath))
	}

	files = files.WithNewFile("checksums.txt", checksums.String())
	return files, nil
}

var notorize = []Archive{
	{Path: "/spektacular_%%VERSION%%_darwin_x86_64.zip", Type: "zip", Output: "/spektacular_%%VERSION%%_darwin_x86_64.zip"},
	{Path: "/spektacular_%%VERSION%%_darwin_arm64.zip", Type: "zip", Output: "/spektacular_%%VERSION%%_darwin_arm64.zip"},
}

// SignAndNotorize signs and notorizes the osx binaries using the Apple notary service
func (d *Spektacular) SignAndNotorize(
	ctx context.Context,
	version string,
	archives *dagger.Directory,
	cert *dagger.File,
	password *dagger.Secret,
	key *dagger.File,
	keyId,
	keyIssuer string,
) (*dagger.Directory, error) {
	if d.hasError() {
		return nil, d.lastError
	}

	cli := dag

	not := dag.Notorize().
		WithP12Cert(cert, password).
		WithNotoryKey(key, keyId, keyIssuer)

	out := archives

	zipContainer := cli.Container().
		From("alpine:latest").
		WithExec([]string{"apk", "add", "zip"})

	for _, a := range notorize {
		path := strings.ReplaceAll(a.Output, "%%VERSION%%", version)

		jpFile := zipContainer.
			WithMountedFile("/spektacular.zip", archives.File(path)).
			WithExec([]string{"unzip", "/spektacular.zip"}).
			File("/spektacular")

		notorized := not.SignAndNotorize(jpFile)

		nFile := zipContainer.
			WithMountedFile("/spektacular", notorized).
			WithExec([]string{"zip", "-r", path, "/spektacular"}).
			File(path)

		out = out.WithFile(path, nFile)
	}

	return out, nil
}

func (d *Spektacular) GithubRelease(
	ctx context.Context,
	src *dagger.Directory,
	archives *dagger.Directory,
	githubToken *dagger.Secret,
) (string, error) {
	if d.hasError() {
		return "", d.lastError
	}

	version, sha, err := d.getVersion(ctx, githubToken, src)
	if err != nil {
		d.lastError = err
		return "", err
	}

	if version == "0.0.0" {
		d.lastError = fmt.Errorf("no version to release, did you tag the PR?")
		return "", d.lastError
	}

	cli := dag

	err = cli.Github().
		WithToken(githubToken).
		CreateRelease(ctx, owner, repo, version, sha, dagger.GithubCreateReleaseOpts{Files: archives})

	if err != nil {
		d.lastError = err
		return "", err
	}

	return version, err
}

func (d *Spektacular) UpdateBrew(
	ctx context.Context,
	version string,
	githubToken *dagger.Secret,
) error {
	if d.hasError() {
		return d.lastError
	}

	cli := dag

	opts := dagger.BrewFormulaOpts{
		DarwinX86Url:   fmt.Sprintf("https://github.com/hivecommons/spektacular/releases/download/%s/spektacular_%s_darwin_x86_64.zip", version, version),
		DarwinArm64Url: fmt.Sprintf("https://github.com/hivecommons/spektacular/releases/download/%s/spektacular_%s_darwin_arm64.zip", version, version),
		LinuxX86Url:    fmt.Sprintf("https://github.com/hivecommons/spektacular/releases/download/%s/spektacular_%s_linux_x86_64.tar.gz", version, version),
		LinuxArm64Url:  fmt.Sprintf("https://github.com/hivecommons/spektacular/releases/download/%s/spektacular_%s_linux_arm64.tar.gz", version, version),
	}

	// Publish to every tap. A failure against one does not skip the rest: a
	// release that updated the canonical tap but not the legacy one is still
	// worth knowing about, and stopping early would hide which taps are now
	// out of step with the release.
	var errs []error
	for _, tap := range brewTaps {
		log.Info("updating brew formula", "tap", tap, "version", version)

		if _, err := cli.Brew().Formula(
			ctx,
			"https://hivecommons.dev",
			tap,
			version,
			"Queen Bee",
			"bee@hivecommons.com",
			"spektacular",
			githubToken,
			opts,
		); err != nil {
			errs = append(errs, fmt.Errorf("updating brew formula in %s: %w", tap, err))
		}
	}

	err := errors.Join(errs...)
	if err != nil {
		d.lastError = err
	}

	return err
}

var gemFury = []Archive{
	{Path: "/pkg/linux/amd64/spektacular.deb", Type: "copy", Output: "spektacular_%%VERSION%%_linux_x86_64.deb"},
	{Path: "/pkg/linux/arm64/spektacular.deb", Type: "copy", Output: "spektacular_%%VERSION%%_linux_arm64.deb"},
}

func (d *Spektacular) UpdateGemFury(
	ctx context.Context,
	version string,
	gemFuryToken *dagger.Secret,
	archives *dagger.Directory,
) error {
	cli := dag

	tkn, _ := gemFuryToken.Plaintext(ctx)
	url := fmt.Sprintf("https://%s@push.fury.io/jumppad/", tkn)

	for _, a := range gemFury {
		output := strings.Replace(a.Output, "%%VERSION%%", version, 1)

		_, err := cli.Container().
			From("curlimages/curl:latest").
			WithFile(output, archives.File(output)).
			WithExec([]string{"-F", fmt.Sprintf("package=@%s", output), url}, dagger.ContainerWithExecOpts{UseEntrypoint: true}).
			Sync(ctx)

		if err != nil {
			d.lastError = err
			return err
		}
	}

	return nil
}

func (d *Spektacular) getVersion(ctx context.Context, token *dagger.Secret, src *dagger.Directory) (string, string, error) {
	if d.hasError() {
		return "", "", d.lastError
	}

	cli := dag

	// get the latest git sha from the source
	ref, err := cli.Container().
		From("alpine/git:v2.49.1").
		WithDirectory("/src", src).
		WithWorkdir("/src").
		WithEnvVariable("GIT_CONFIG_GLOBAL", "/tmp/gitconfig").
		WithNewFile("/tmp/gitconfig", "[safe]\n\tdirectory = *\n").
		WithExec([]string{"rev-parse", "HEAD"}, dagger.ContainerWithExecOpts{UseEntrypoint: true}).
		Stdout(ctx)

	if err != nil {
		d.lastError = err
		return "", "", err
	}

	// make sure there is no whitespace from the output
	ref = strings.TrimSpace(ref)
	log.Info("github reference", "sha", ref)

	// get the next version from the associated PR label
	v, err := cli.Github().
		WithToken(token).
		NextVersionFromAssociatedPrlabel(ctx, owner, repo, ref)

	if err != nil {
		d.lastError = err
		return "", "", err
	}

	// if there is no version, default to 0.0.0
	if v == "" {
		v = "0.0.0"
	}

	log.Info("new version", "semver", v)

	return v, ref, nil
}

func (d *Spektacular) goCache() *dagger.CacheVolume {
	if d.goCacheVolume == nil {
		d.goCacheVolume = dag.CacheVolume("go-cache")
	}

	return d.goCacheVolume
}

func (d *Spektacular) hasError() bool {
	return d.lastError != nil
}

func (d *Spektacular) setArchLocalMachine(ctx context.Context) {
	// get the architecture of the current machine
	platform, err := dag.DefaultPlatform(ctx)
	if err != nil {
		panic(err)
	}

	arch := strings.Split(string(platform), "/")[1]
	os := strings.Split(string(platform), "/")[0]

	fmt.Println("Set build add for arch:", arch)

	oses = []string{os}
	arches = []string{arch}

	outputArch := arch
	if outputArch == "amd64" {
		outputArch = "x86_64"
	}

	// only change notorize if we are on darwin
	if os == "darwin" {
		filename := strings.Replace("spektacular_%%VERSION%%_darwin_%%ARCH%%.zip", "%%ARCH%%", outputArch, 1)
		notorize = []Archive{
			{Path: filename, Type: "zip", Output: filename},
		}

		archives = []Archive{
			{Path: fmt.Sprintf("/build/darwin/%s/spektacular.exe", arch), Type: "zip", Output: filename},
		}
	}

	if os == "linux" {
		filename := strings.Replace("spektacular_%%VERSION%%_darwin_%%ARCH%%.tar.gz", "%%ARCH%%", outputArch, 1)
		filenameDeb := strings.Replace("spektacular_%%VERSION%%_darwin_%%ARCH%%.deb", "%%ARCH%%", outputArch, 1)

		archives = []Archive{
			{Path: fmt.Sprintf("/build/linux/%s/spektacular", arch), Type: "targz", Output: filename},
			{Path: fmt.Sprintf("/pkg/linux/%s/spektacular.deb", arch), Type: "copy", Output: filenameDeb},
		}
	}
}
