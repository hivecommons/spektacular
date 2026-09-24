package identifier

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

func TestHasPrefix_Counter(t *testing.T) {
	require.True(t, HasPrefix(MethodCounter, "000034_feature"))
	require.False(t, HasPrefix(MethodCounter, "feature"))
	require.False(t, HasPrefix(MethodCounter, "34_feature"), "must be exactly six digits")
	require.False(t, HasPrefix(MethodCounter, "20260709000000-feature"), "timestamp-shaped id must not satisfy counter")
}

func TestHasPrefix_Timestamp(t *testing.T) {
	require.True(t, HasPrefix(MethodTimestamp, "20260709000000-feature"))
	require.False(t, HasPrefix(MethodTimestamp, "feature"))
	require.False(t, HasPrefix(MethodTimestamp, "000034_feature"), "counter-shaped id must not satisfy timestamp")
}

func TestHasPrefix_External(t *testing.T) {
	require.True(t, HasPrefix(MethodExternal, "feature"), "external ids have no fixed shape to check mechanically")
}

func TestHasPrefix_EmptyMethodDefaultsToTimestamp(t *testing.T) {
	require.True(t, HasPrefix("", "20260709000000-feature"))
	require.False(t, HasPrefix("", "feature"))
}

// docPath is a stand-in PathFunc used by these tests, shaped like
// spec.SpecFilePath (dir + "/" + name + ".md") without importing the spec
// package — this package must not depend on any of its callers.
func docPath(dir, name string) string {
	return filepath.Join(dir, name+".md")
}

func identifierStore(t *testing.T) store.Store {
	t.Helper()
	return store.NewFileStore(t.TempDir(), "project")
}

func writeExistingDoc(t *testing.T, st store.Store, name string) {
	t.Helper()
	writeExistingDocIn(t, st, config.DefaultSpecDir, name)
}

func writeExistingDocIn(t *testing.T, st store.Store, dir, name string) {
	t.Helper()
	require.NoError(t, st.Write(docPath(dir, name), []byte("existing")))
}

func fixedResolveTime() time.Time {
	return time.Date(2026, time.May, 8, 21, 2, 3, 0, time.FixedZone("EDT", -4*60*60))
}

func TestResolve_DefaultTimestamp(t *testing.T) {
	st := identifierStore(t)

	got, err := Resolve(Request{
		Name:     "Billing.Export",
		Store:    st,
		PathFunc: docPath,
		Now:      fixedResolveTime,
	})

	require.NoError(t, err)
	require.Equal(t, "20260509010203-billing-export", got.Name)
}

func TestResolve_TimestampCollisionBumpsSeconds(t *testing.T) {
	st := identifierStore(t)
	writeExistingDoc(t, st, "20260509010203-billing-export")

	got, err := Resolve(Request{
		Name:     "billing-export",
		Method:   MethodTimestamp,
		Store:    st,
		PathFunc: docPath,
		Now:      fixedResolveTime,
	})

	require.NoError(t, err)
	require.Equal(t, "20260509010204-billing-export", got.Name)
}

func TestResolve_ExplicitTimestampMethod(t *testing.T) {
	st := identifierStore(t)

	got, err := Resolve(Request{
		Name:     "Billing@Export",
		Method:   MethodTimestamp,
		Store:    st,
		PathFunc: docPath,
		Now:      fixedResolveTime,
	})

	require.NoError(t, err)
	require.Equal(t, "20260509010203-billing-export", got.Name)
}

func TestResolve_CounterEmptyStoreStartsAtOne(t *testing.T) {
	st := identifierStore(t)

	got, err := Resolve(Request{
		Name:     "billing-export",
		Method:   MethodCounter,
		Store:    st,
		PathFunc: docPath,
	})

	require.NoError(t, err)
	require.Equal(t, "000001_billing-export", got.Name)
}

func TestResolve_CounterUsesMaxFromStorePlusOne(t *testing.T) {
	st := identifierStore(t)
	writeExistingDoc(t, st, "000007_old-feature")

	got, err := Resolve(Request{
		Name:     "billing-export",
		Method:   MethodCounter,
		Store:    st,
		PathFunc: docPath,
	})

	require.NoError(t, err)
	require.Equal(t, "000008_billing-export", got.Name)
}

func TestResolve_CounterIgnoresTimestampAndExternalPrefixes(t *testing.T) {
	st := identifierStore(t)
	writeExistingDoc(t, st, "000003_old-feature")
	writeExistingDoc(t, st, "20260509010203-other-feature")
	writeExistingDoc(t, st, "ext-123-external")

	got, err := Resolve(Request{
		Name:     "billing-export",
		Method:   MethodCounter,
		Store:    st,
		PathFunc: docPath,
	})

	require.NoError(t, err)
	require.Equal(t, "000004_billing-export", got.Name)
}

func TestResolve_CounterCollisionBumpsValue(t *testing.T) {
	st := identifierStore(t)
	writeExistingDoc(t, st, "000007_old-feature")
	writeExistingDoc(t, st, "000008_billing-export")

	got, err := Resolve(Request{
		Name:     "billing-export",
		Method:   MethodCounter,
		Store:    st,
		PathFunc: docPath,
	})

	require.NoError(t, err)
	require.Equal(t, "000009_billing-export", got.Name)
}

// TestResolve_CounterEnumeratesConfiguredDir asserts that counter resolution
// scans the configured (non-default) directory for existing documents.
func TestResolve_CounterEnumeratesConfiguredDir(t *testing.T) {
	st := identifierStore(t)
	// Existing counter-prefixed doc under the non-default directory.
	writeExistingDocIn(t, st, "my-specs", "000007_old-feature")
	// A decoy under the default "specs" dir must be ignored.
	writeExistingDocIn(t, st, "specs", "000099_decoy")

	got, err := Resolve(Request{
		Name:     "billing-export",
		Method:   MethodCounter,
		Dir:      "my-specs",
		Store:    st,
		PathFunc: docPath,
	})

	require.NoError(t, err)
	require.Equal(t, "000008_billing-export", got.Name)
}

func TestResolve_ExplicitIDRejectedUnderGeneratedMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{name: "default timestamp"},
		{name: "timestamp", method: MethodTimestamp},
		{name: "counter", method: MethodCounter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := identifierStore(t)

			_, err := Resolve(Request{
				Name:     "Billing Export",
				ID:       "36",
				Method:   tt.method,
				Store:    st,
				PathFunc: docPath,
			})

			var notAllowed *IDNotAllowedError
			require.ErrorAs(t, err, &notAllowed)
		})
	}
}

func TestResolve_ExplicitIDCollisionFails(t *testing.T) {
	st := identifierStore(t)
	writeExistingDoc(t, st, "ext-123-billing")

	_, err := Resolve(Request{
		Name:     "billing",
		ID:       "EXT.123",
		Method:   MethodExternal,
		Store:    st,
		PathFunc: docPath,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestResolve_ExternalRequiresID(t *testing.T) {
	st := identifierStore(t)

	_, err := Resolve(Request{
		Name:     "billing",
		Method:   MethodExternal,
		Store:    st,
		PathFunc: docPath,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "id is required")
}

func TestResolve_ExternalUsesNormalizedID(t *testing.T) {
	st := identifierStore(t)

	got, err := Resolve(Request{
		Name:     "Billing Export",
		ID:       "EXT.User@123",
		Method:   MethodExternal,
		Store:    st,
		PathFunc: docPath,
	})

	require.NoError(t, err)
	require.Equal(t, "ext-user-123-billing-export", got.Name)
}

func TestResolve_UnknownMethodReturnsError(t *testing.T) {
	st := identifierStore(t)

	_, err := Resolve(Request{
		Name:     "billing",
		Method:   "unsupported",
		Store:    st,
		PathFunc: docPath,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported id_method")
}

func TestResolve_RejectsUnsafeOrUntrimmedValuesBeforeStoreUse(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{
			name: "leading whitespace name",
			req:  Request{Name: " billing", Method: MethodTimestamp},
			want: "leading or trailing whitespace",
		},
		{
			name: "trailing whitespace id",
			req:  Request{Name: "billing", ID: "ext ", Method: MethodTimestamp},
			want: "leading or trailing whitespace",
		},
		{
			name: "path separator name",
			req:  Request{Name: "billing/export", Method: MethodTimestamp},
			want: "path separators",
		},
		{
			name: "control character id",
			req:  Request{Name: "billing", ID: "bad\nid", Method: MethodTimestamp},
			want: "control characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.req)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestResolve_NilStoreFailsGeneratedModes(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{name: "default timestamp"},
		{name: "timestamp", method: MethodTimestamp},
		{name: "counter", method: MethodCounter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(Request{
				Name:     "billing",
				Method:   tt.method,
				PathFunc: docPath,
				Now:      fixedResolveTime,
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "store required")
		})
	}
}
