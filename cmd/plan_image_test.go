package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	imageTestFeature = "20260709000000-feature"
	imageTestSVG     = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40" viewBox="0 0 120 40">
  <rect x="1" y="1" width="118" height="38" rx="6" fill="none" stroke="#333"/>
  <text x="60" y="25" text-anchor="middle">Start</text>
</svg>
`
)

// writeImagePlanFixture lays out a project whose plan directory is
// docs/plans (and chdirs into it), seeds one plan document for the test
// feature, and returns the project directory and an SVG source file.
func writeImagePlanFixture(t *testing.T) (dir, svgPath string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")
	planPath := filepath.Join(dir, "docs", "plans", imageTestFeature, "plan.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(planPath), 0o755))
	require.NoError(t, os.WriteFile(planPath, []byte("plan body"), 0o644))
	svgPath = filepath.Join(t.TempDir(), "flow.svg")
	require.NoError(t, os.WriteFile(svgPath, []byte(imageTestSVG), 0o644))
	return dir, svgPath
}

func decodeOutput(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got), "stdout must be one JSON document: %s", stdout)
	return got
}

// A write stores the SVG byte for byte beside the plan's documents, with no
// front matter, and reports the relative link plan.md uses to show it.
func TestPlanImageWrite_StoresSVGVerbatimAndReportsLink(t *testing.T) {
	dir, svgPath := writeImagePlanFixture(t)

	stdout, _, code := runRootCmd(t, "plan", "image", "write", imageTestFeature, "login-flow", "--from", svgPath)
	require.Equal(t, 0, code, stdout)

	stored, err := os.ReadFile(filepath.Join(dir, "docs", "plans", imageTestFeature, "images", "login-flow.svg"))
	require.NoError(t, err)
	require.Equal(t, imageTestSVG, string(stored), "an image must be stored exactly as supplied")

	got := decodeOutput(t, stdout)
	require.Equal(t, "login-flow", got["name"])
	require.Equal(t, "images/login-flow.svg", got["link"])
	require.Equal(t, "../docs/plans/"+imageTestFeature+"/images/login-flow.svg", got["path"])
}

func TestPlanImageRead_ReturnsStoredBytes(t *testing.T) {
	_, svgPath := writeImagePlanFixture(t)
	_, _, code := runRootCmd(t, "plan", "image", "write", imageTestFeature, "login-flow", "--from", svgPath)
	require.Equal(t, 0, code)

	stdout, _, code := runRootCmd(t, "plan", "image", "read", imageTestFeature, "login-flow")
	require.Equal(t, 0, code)
	require.Equal(t, imageTestSVG, stdout)
}

func TestPlanImageRead_MissingImageIsNotFound(t *testing.T) {
	writeImagePlanFixture(t)

	stdout, _, code := runRootCmd(t, "plan", "image", "read", imageTestFeature, "nosuch")
	require.NotEqual(t, 0, code)
	got := decodeOutput(t, stdout)
	require.Equal(t, "not_found", got["code"])
	require.Contains(t, got["next_action"], "plan image list "+imageTestFeature)
}

// List names only the stored SVGs, never the plan's Markdown documents, and
// `plan file list` never reports the images folder as a document.
func TestPlanImageList_NamesImagesOnly(t *testing.T) {
	dir, svgPath := writeImagePlanFixture(t)
	for _, name := range []string{"mock-up", "login-flow"} {
		_, _, code := runRootCmd(t, "plan", "image", "write", imageTestFeature, name, "--from", svgPath)
		require.Equal(t, 0, code)
	}
	stray := filepath.Join(dir, "docs", "plans", imageTestFeature, "images", "notes.txt")
	require.NoError(t, os.WriteFile(stray, []byte("x"), 0o644))

	stdout, _, code := runRootCmd(t, "plan", "image", "list", imageTestFeature)
	require.Equal(t, 0, code, stdout)
	images := decodeOutput(t, stdout)["images"].([]any)
	names := []string{}
	for _, img := range images {
		entry := img.(map[string]any)
		names = append(names, entry["name"].(string))
		require.Equal(t, "images/"+entry["name"].(string)+".svg", entry["link"])
	}
	require.ElementsMatch(t, []string{"mock-up", "login-flow"}, names)

	stdout, _, code = runRootCmd(t, "plan", "file", "list", imageTestFeature)
	require.Equal(t, 0, code, stdout)
	require.NotContains(t, stdout, `"images"`, "the images folder must not be listed as a plan document")
}

func TestPlanImageList_PlanWithNoImagesIsEmpty(t *testing.T) {
	writeImagePlanFixture(t)

	stdout, _, code := runRootCmd(t, "plan", "image", "list", imageTestFeature)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, decodeOutput(t, stdout)["images"])
}

func TestPlanImageList_UnknownPlanIsNotFound(t *testing.T) {
	writeImagePlanFixture(t)

	stdout, _, code := runRootCmd(t, "plan", "image", "list", "20260709000000-nosuch")
	require.NotEqual(t, 0, code)
	require.Equal(t, "not_found", decodeOutput(t, stdout)["code"])
}

func TestPlanImageDelete_RemovesImage(t *testing.T) {
	dir, svgPath := writeImagePlanFixture(t)
	_, _, code := runRootCmd(t, "plan", "image", "write", imageTestFeature, "login-flow", "--from", svgPath)
	require.Equal(t, 0, code)

	_, _, code = runRootCmd(t, "plan", "image", "delete", imageTestFeature, "login-flow")
	require.Equal(t, 0, code)
	require.NoFileExists(t, filepath.Join(dir, "docs", "plans", imageTestFeature, "images", "login-flow.svg"))
}

// A source that is not an SVG document is refused before anything is
// written: raster images, HTML mock-ups and malformed markup alike.
func TestPlanImageWrite_RefusesNonSVG(t *testing.T) {
	cases := map[string]string{
		"png":       "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR",
		"html":      "<html><body><button>Save</button></body></html>",
		"malformed": "<svg xmlns=\"http://www.w3.org/2000/svg\"><rect></svg>",
		"empty":     "",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir, _ := writeImagePlanFixture(t)
			src := filepath.Join(t.TempDir(), "source")
			require.NoError(t, os.WriteFile(src, []byte(body), 0o644))

			stdout, _, code := runRootCmd(t, "plan", "image", "write", imageTestFeature, "mock-up", "--from", src)
			require.NotEqual(t, 0, code)
			require.Equal(t, "image_not_svg", decodeOutput(t, stdout)["code"])
			require.NoDirExists(t, filepath.Join(dir, "docs", "plans", imageTestFeature, "images"))
		})
	}
}

// An image name carrying an extension or a path is refused with the
// command correctly spelled.
func TestPlanImageWrite_RefusesExtensionWithCorrection(t *testing.T) {
	dir, svgPath := writeImagePlanFixture(t)

	stdout, _, code := runRootCmd(t, "plan", "image", "write", imageTestFeature, "images/login-flow.svg", "--from", svgPath)
	require.NotEqual(t, 0, code)
	got := decodeOutput(t, stdout)
	require.Equal(t, "unexpected_extension", got["code"])
	require.Contains(t, got["next_action"], "plan image write "+imageTestFeature+" login-flow --from")
	require.NoDirExists(t, filepath.Join(dir, "docs", "plans", imageTestFeature, "images"))
}

// Images belong to a plan, so a write reuses the plan's spec ID exactly as a
// plan document write does.
func TestPlanImageWrite_RequiresIDPrefix(t *testing.T) {
	_, svgPath := writeImagePlanFixture(t)

	stdout, _, code := runRootCmd(t, "plan", "image", "write", "feature", "login-flow", "--from", svgPath)
	require.NotEqual(t, 0, code)
	require.Equal(t, "missing_id_prefix", decodeOutput(t, stdout)["code"])
}

func TestValidateSVG(t *testing.T) {
	require.NoError(t, validateSVG([]byte(imageTestSVG)))
	require.NoError(t, validateSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)))
	require.ErrorContains(t, validateSVG([]byte(`<div/>`)), "<div>")
	require.Error(t, validateSVG([]byte(`<svg><g></svg>`)))
	require.Error(t, validateSVG([]byte(`just text`)))
}
