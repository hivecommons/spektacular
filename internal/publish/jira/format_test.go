package jira

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish"
)

func TestFormat_ADFAndWikiKeepStructureAndMarker(t *testing.T) {
	md := "# Title\n\nA paragraph with `code`, **strong** and [a link](https://x.test).\nSecond line.\n\n## Children\n\n- [ ] DEMO-2 — One\n- [ ] DEMO-3 — Two\n\n```\nraw\n```\n\n<!-- spek:publish name=n artifact_id=a/spec/n target=jira -->\n"
	doc := toADF(md)
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	text := ADFText(decoded)
	require.Contains(t, text, "Title\n")
	require.Contains(t, text, "A paragraph with code, strong and a link. Second line.\n")
	require.Contains(t, text, "DEMO-2 — One\n")
	require.Contains(t, text, "raw\n")
	m, ok := publish.ParseMarker(text)
	require.True(t, ok)
	require.Equal(t, "a/spec/n", m.ArtifactID)
	require.Contains(t, string(raw), `"type":"link"`)
	require.Contains(t, string(raw), `"type":"codeBlock"`)
	require.Contains(t, string(raw), `"type":"bulletList"`)

	wiki := toWiki(md)
	require.Contains(t, wiki, "h1. Title\n")
	require.Contains(t, wiki, "{{code}}, *strong* and [a link|https://x.test]")
	require.Contains(t, wiki, "* [ ] DEMO-2 — One\n")
	require.Contains(t, wiki, "{code}\nraw\n{code}")
	_, ok = publish.ParseMarker(wiki)
	require.True(t, ok)
}
