package migrate

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Helpers for editing a settings file as a yaml.v3 node tree. Steps work on
// nodes rather than the typed config structs so comments, key order and
// unrelated keys in hand-edited files survive an upgrade.

// parseDoc parses raw settings bytes into a document node. An empty file
// becomes a document holding an empty mapping.
func parseDoc(raw []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("settings file is not a YAML mapping")
	}
	return &doc, nil
}

// docRoot returns the top-level mapping of a document node.
func docRoot(doc *yaml.Node) *yaml.Node {
	return doc.Content[0]
}

// lookup returns the value node for key in mapping m, and the index of its key
// node, or (nil, -1) when absent.
func lookup(m *yaml.Node, key string) (*yaml.Node, int) {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil, -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1], i
		}
	}
	return nil, -1
}

// getNode walks a dotted path ("spec.config.directory") from mapping root.
func getNode(root *yaml.Node, path string) *yaml.Node {
	n := root
	for _, key := range strings.Split(path, ".") {
		n, _ = lookup(n, key)
		if n == nil {
			return nil
		}
	}
	return n
}

// getScalar returns the scalar value at a dotted path.
func getScalar(root *yaml.Node, path string) (string, bool) {
	n := getNode(root, path)
	if n == nil || n.Kind != yaml.ScalarNode {
		return "", false
	}
	return n.Value, true
}

// setScalar sets the string value at a dotted path, creating intermediate
// mappings as needed. An existing scalar keeps its position and comments.
func setScalar(root *yaml.Node, path, value string) {
	setNode(root, path, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

// setNode places value at a dotted path, creating intermediate mappings as
// needed and appending new keys at the end of their mapping.
func setNode(root *yaml.Node, path string, value *yaml.Node) {
	keys := strings.Split(path, ".")
	m := root
	for i, key := range keys {
		v, idx := lookup(m, key)
		last := i == len(keys)-1
		if last {
			if idx >= 0 {
				if v.Kind == yaml.ScalarNode && value.Kind == yaml.ScalarNode {
					v.Tag, v.Value, v.Style = value.Tag, value.Value, 0
				} else {
					m.Content[idx+1] = value
				}
			} else {
				m.Content = append(m.Content, scalarKey(key), value)
			}
			return
		}
		if v == nil || v.Kind != yaml.MappingNode {
			child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			if idx >= 0 {
				m.Content[idx+1] = child
			} else {
				m.Content = append(m.Content, scalarKey(key), child)
			}
			v = child
		}
		m = v
	}
}

// deleteKey removes a top-level-relative dotted key when present.
func deleteKey(root *yaml.Node, path string) {
	keys := strings.Split(path, ".")
	parent := root
	if len(keys) > 1 {
		parent = getNode(root, strings.Join(keys[:len(keys)-1], "."))
	}
	if _, idx := lookup(parent, keys[len(keys)-1]); idx >= 0 {
		parent.Content = append(parent.Content[:idx], parent.Content[idx+2:]...)
	}
}

// setSchema records the format version, inserting `schema` as the file's
// first key when it is absent.
func setSchema(root *yaml.Node, n int) {
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(n)}
	if v, _ := lookup(root, "schema"); v != nil {
		v.Kind, v.Tag, v.Value, v.Style = yaml.ScalarNode, val.Tag, val.Value, 0
		return
	}
	root.Content = append([]*yaml.Node{scalarKey("schema"), val}, root.Content...)
}

// setWrittenBy records the writing Spektacular version, directly after
// `schema` when it is absent.
func setWrittenBy(root *yaml.Node, version string) {
	if v, _ := lookup(root, "written_by"); v != nil {
		v.Kind, v.Tag, v.Value, v.Style = yaml.ScalarNode, "!!str", version, 0
		return
	}
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: version}
	at := 0
	if _, idx := lookup(root, "schema"); idx >= 0 {
		at = idx + 2
	}
	content := append([]*yaml.Node{}, root.Content[:at]...)
	content = append(content, scalarKey("written_by"), val)
	root.Content = append(content, root.Content[at:]...)
}

func scalarKey(key string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
}

// encode serialises a document node with the 4-space indent yaml.Marshal
// uses, so upgraded files match files written by the config package.
func encode(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
