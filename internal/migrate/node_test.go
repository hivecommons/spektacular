package migrate

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func encodeString(t *testing.T, raw string, edit func(root *yaml.Node)) string {
	t.Helper()
	doc, err := parseDoc([]byte(raw))
	require.NoError(t, err)
	edit(docRoot(doc))
	out, err := encode(doc)
	require.NoError(t, err)
	return string(out)
}

func TestSetScalar_CreatesIntermediateMappings(t *testing.T) {
	out := encodeString(t, "a: 1\n", func(root *yaml.Node) {
		setScalar(root, "b.c.d", "x")
	})
	require.Equal(t, "a: 1\nb:\n    c:\n        d: x\n", out)
}

func TestSetScalar_ReplacesExistingValueInPlace(t *testing.T) {
	out := encodeString(t, "a: 1\nb: old\nc: 3\n", func(root *yaml.Node) {
		setScalar(root, "b", "new")
	})
	require.Equal(t, "a: 1\nb: new\nc: 3\n", out)
}

func TestSetSchema_InsertsAsFirstKey(t *testing.T) {
	out := encodeString(t, "name: x\nagent: claude\n", func(root *yaml.Node) {
		setSchema(root, 2)
	})
	require.Equal(t, "schema: 2\nname: x\nagent: claude\n", out)
}

func TestSetSchema_UpdatesExistingValue(t *testing.T) {
	out := encodeString(t, "name: x\nschema: 1\n", func(root *yaml.Node) {
		setSchema(root, 3)
	})
	require.Equal(t, "name: x\nschema: 3\n", out)
}

func TestSetWrittenBy_InsertsDirectlyAfterSchema(t *testing.T) {
	out := encodeString(t, "name: x\n", func(root *yaml.Node) {
		setSchema(root, 2)
		setWrittenBy(root, "0.1.0")
	})
	require.Equal(t, "schema: 2\nwritten_by: 0.1.0\nname: x\n", out)
}

func TestEncode_PreservesComments(t *testing.T) {
	raw := "name: x\n# about the agent\nagent: claude # inline\n"
	out := encodeString(t, raw, func(root *yaml.Node) {
		setScalar(root, "agent", "bob")
	})
	require.Equal(t, "name: x\n# about the agent\nagent: bob # inline\n", out)
}

func TestDeleteKey_RemovesNestedKey(t *testing.T) {
	out := encodeString(t, "a:\n    b: 1\n    c: 2\n", func(root *yaml.Node) {
		deleteKey(root, "a.b")
		deleteKey(root, "missing")
	})
	require.Equal(t, "a:\n    c: 2\n", out)
}

func TestParseDoc_EmptyFileIsEmptyMapping(t *testing.T) {
	doc, err := parseDoc(nil)
	require.NoError(t, err)
	_, ok := getScalar(docRoot(doc), "schema")
	require.False(t, ok)
}

func TestParseDoc_RejectsNonMapping(t *testing.T) {
	_, err := parseDoc([]byte("- a\n- b\n"))
	require.Error(t, err)
}
