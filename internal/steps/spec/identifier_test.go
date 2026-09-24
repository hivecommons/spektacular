package spec

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// All ID-shape and minting rules are tested in internal/identifier. This
// file only proves ResolveIdentifier wires SpecFilePath through as the path
// existence checks resolve against — the one thing that is actually
// spec-specific about this adapter.

func fixedIdentifierTime() time.Time {
	return time.Date(2026, time.May, 8, 21, 2, 3, 0, time.FixedZone("EDT", -4*60*60))
}

func TestResolveIdentifier_UsesSpecFilePathForExistenceChecks(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	require.NoError(t, st.Write(SpecFilePath("specs", "20260509010203-a1b2c3d4-billing-export"), []byte("existing")))

	got, err := ResolveIdentifier(IdentifierRequest{
		Name:    "billing-export",
		SpecDir: "specs",
		Store:   st,
		Now:     fixedIdentifierTime,
		RandomID: func() (string, error) {
			return "a1b2c3d4", nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, "20260509010204-a1b2c3d4-billing-export", got.Name, "must have detected the collision via SpecFilePath and bumped the timestamp")
}

func TestResolveIdentifier_CounterEnumeratesConfiguredSpecDir(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	require.NoError(t, st.Write(SpecFilePath("my-specs", "000007_old-feature"), []byte("existing")))
	require.NoError(t, st.Write(SpecFilePath("specs", "000099_decoy"), []byte("existing")))

	got, err := ResolveIdentifier(IdentifierRequest{
		Name:    "billing-export",
		Method:  "counter",
		SpecDir: "my-specs",
		Store:   st,
	})

	require.NoError(t, err)
	require.Equal(t, "000008_billing-export", got.Name)
}
