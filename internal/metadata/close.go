package metadata

// StoreReadWriter is the minimal store surface Close needs. Any store type
// satisfying it — e.g. `internal/store.FileStore` — can be passed without
// forcing this package to import a concrete store implementation.
type StoreReadWriter interface {
	Read(path string) ([]byte, error)
	Write(path string, content []byte) error
}

// Close transitions the artifact at path to the given document status by rewriting its
// frontmatter block only — the body is preserved byte-for-byte. Malformed
// existing frontmatter propagates as an error rather than being silently
// replaced. Callers that want to no-op on a missing artifact should check the
// returned error against their store's not-found sentinel.
func Close(st StoreReadWriter, path string, status DocumentStatus) error {
	existing, err := st.Read(path)
	if err != nil {
		return err
	}
	body := existing
	if _, split, splitErr := Split(existing); splitErr == nil {
		body = split
	}
	merged, err := Merge(existing, body, UpdateOptions{DocumentStatus: &status})
	if err != nil {
		return err
	}
	return st.Write(path, merged)
}
