package depgraph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindCycle_AcyclicReturnsNil(t *testing.T) {
	require.Nil(t, FindCycle(nil, nil))
	require.Nil(t, FindCycle([]string{}, map[string][]string{}))
	require.Nil(t, FindCycle([]string{"a"}, nil))

	// A diamond: d is reached twice but never revisited while on the stack.
	order := []string{"a", "b", "c", "d"}
	deps := map[string][]string{
		"a": {"b", "c"},
		"b": {"d"},
		"c": {"d"},
	}
	require.Nil(t, FindCycle(order, deps))
}

func TestFindCycle_SelfLoop(t *testing.T) {
	order := []string{"a"}
	deps := map[string][]string{"a": {"a"}}
	require.Equal(t, []string{"a", "a"}, FindCycle(order, deps))
}

func TestFindCycle_TwoNodeCycle(t *testing.T) {
	order := []string{"a", "b"}
	deps := map[string][]string{"a": {"b"}, "b": {"a"}}
	require.Equal(t, []string{"a", "b", "a"}, FindCycle(order, deps))
}

func TestFindCycle_LongerCyclePath(t *testing.T) {
	order := []string{"a", "b", "c"}
	deps := map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}}
	require.Equal(t, []string{"a", "b", "c", "a"}, FindCycle(order, deps))
}

func TestFindCycle_PathExcludesNodesLeadingIntoCycle(t *testing.T) {
	order := []string{"x", "a", "b", "c"}
	deps := map[string][]string{"x": {"a"}, "a": {"b"}, "b": {"c"}, "c": {"a"}}
	require.Equal(t, []string{"a", "b", "c", "a"}, FindCycle(order, deps))
}

func TestFindCycle_IgnoresUnknownDependencies(t *testing.T) {
	order := []string{"a", "b"}
	deps := map[string][]string{
		"a": {"ghost", "b"},
		"b": {"phantom"},
		// A cycle among names not in order is not reported either.
		"ghost":   {"phantom"},
		"phantom": {"ghost"},
	}
	require.Nil(t, FindCycle(order, deps))
}

func TestFindCycle_DeclarationOrderDecidesReportedCycle(t *testing.T) {
	deps := map[string][]string{
		"a": {"b"}, "b": {"a"},
		"c": {"d"}, "d": {"c"},
	}
	require.Equal(t, []string{"a", "b", "a"}, FindCycle([]string{"a", "b", "c", "d"}, deps))
	require.Equal(t, []string{"c", "d", "c"}, FindCycle([]string{"c", "d", "a", "b"}, deps))
	require.Equal(t, []string{"d", "c", "d"}, FindCycle([]string{"d", "c", "b", "a"}, deps))

	// Repeated calls on the same input report the same cycle.
	order := []string{"c", "d", "a", "b"}
	first := FindCycle(order, deps)
	for range 20 {
		require.Equal(t, first, FindCycle(order, deps))
	}
}
