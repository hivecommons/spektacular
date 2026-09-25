package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// rankingConstantDocs are the documents in this repository that quote the
// ranking constants by value. The published website page quotes them too, but
// lives in another repository and cannot be checked from here.
var rankingConstantDocs = []string{
	"../../docs/knowledge-base.md",
	"../../.spektacular/knowledge/architecture/knowledge-search-ranking.md",
}

// Every document that quotes a ranking constant writes it as `name = value`.
// This test holds each such statement to the value in ranking.go, so tuning a
// weight without updating the prose fails the build instead of leaving the
// documentation silently stale.
func TestRankingConstants_MatchDocumentation(t *testing.T) {
	constants := map[string]float64{
		"tagWeight":        tagWeight,
		"minPrefixLen":     minPrefixLen,
		"coverageExponent": coverageExponent,
		"cutoffFraction":   cutoffFraction,
	}

	for _, doc := range rankingConstantDocs {
		t.Run(filepath.Base(doc), func(t *testing.T) {
			body, err := os.ReadFile(doc)
			require.NoError(t, err)

			for name, want := range constants {
				re := regexp.MustCompile(fmt.Sprintf("`%s = ([0-9.]+)`", name))
				matches := re.FindAllStringSubmatch(string(body), -1)
				require.NotEmpty(t, matches, "%s no longer states %s; update this test if that is deliberate", doc, name)

				for _, m := range matches {
					got, err := strconv.ParseFloat(m[1], 64)
					require.NoError(t, err)
					require.Equal(t, want, got, "%s states %s = %s but ranking.go has %v", doc, name, m[1], want)
				}
			}
		})
	}
}
