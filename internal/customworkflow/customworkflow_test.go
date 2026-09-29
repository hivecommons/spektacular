package customworkflow

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

type conditionData map[string]any

func (d conditionData) Get(key string) (any, bool) { v, ok := d[key]; return v, ok }
func (d conditionData) Set(key string, value any)  { d[key] = value }

func TestConditionMatchesExists(t *testing.T) {
	exists := true
	missing := false
	data := conditionData{"channel": "email"}

	require.True(t, (Condition{Key: "channel", Exists: &exists}).Matches(data))
	require.False(t, (Condition{Key: "missing", Exists: &exists}).Matches(data))
	require.True(t, (Condition{Key: "missing", Exists: &missing}).Matches(data))
	require.False(t, (Condition{Key: "channel", Exists: &missing}).Matches(data))
}

func TestConditionMatchesTypedEquality(t *testing.T) {
	data := conditionData{
		"enabled": true,
		"count":   float64(3),
		"channel": "email",
	}

	require.True(t, (Condition{Key: "enabled", Equals: true}).Matches(data))
	require.False(t, (Condition{Key: "enabled", Equals: "true"}).Matches(data))
	require.True(t, (Condition{Key: "count", Equals: 3}).Matches(data), "YAML integer conditions should match JSON numeric workflow data")
	require.True(t, (Condition{Key: "channel", NotEquals: "blog"}).Matches(data))
	require.False(t, (Condition{Key: "channel", NotEquals: "email"}).Matches(data))
}

func TestReadDefinitionRejectsUnknownFields(t *testing.T) {
	_, _, err := readDefinition(fstest.MapFS{
		"workflow.yaml": &fstest.MapFile{Data: []byte(`name: typo-workflow
steps:
  - name: one
    prompt: one.md
    transitons:
      - to: two
`)},
		"one.md": &fstest.MapFile{Data: []byte("one")},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "field transitons not found")
}
