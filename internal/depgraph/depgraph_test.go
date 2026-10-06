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

func TestCycleMembers_AcyclicReturnsNil(t *testing.T) {
	require.Nil(t, CycleMembers(nil, nil))
	require.Nil(t, CycleMembers([]string{}, map[string][]string{}))
	require.Nil(t, CycleMembers([]string{"a", "b"}, nil))

	order := []string{"a", "b", "c", "d"}
	deps := map[string][]string{"a": {"b", "c"}, "b": {"d"}, "c": {"d"}}
	require.Nil(t, CycleMembers(order, deps))
}

func TestCycleMembers_SelfLoop(t *testing.T) {
	order := []string{"a", "b"}
	deps := map[string][]string{"a": {"a"}}
	require.Equal(t, []string{"a"}, CycleMembers(order, deps))
}

func TestCycleMembers_TwoCycle(t *testing.T) {
	order := []string{"a", "b", "c"}
	deps := map[string][]string{"a": {"b"}, "b": {"a"}}
	require.Equal(t, []string{"a", "b"}, CycleMembers(order, deps))
}

func TestCycleMembers_ThreeCycle(t *testing.T) {
	order := []string{"a", "b", "c"}
	deps := map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}}
	require.Equal(t, []string{"a", "b", "c"}, CycleMembers(order, deps))
}

func TestCycleMembers_TwoDisjointCycles(t *testing.T) {
	order := []string{"a", "b", "x", "c", "d"}
	deps := map[string][]string{
		"a": {"b"}, "b": {"a"},
		"c": {"d"}, "d": {"c"},
	}
	require.Equal(t, []string{"a", "b", "c", "d"}, CycleMembers(order, deps))
}

func TestCycleMembers_ReportedInOrdersOrder(t *testing.T) {
	order := []string{"c", "a", "b"}
	deps := map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}}
	require.Equal(t, []string{"c", "a", "b"}, CycleMembers(order, deps))
}

func TestCycleMembers_DependentOfCycleNotReported(t *testing.T) {
	order := []string{"d", "a", "b", "e"}
	deps := map[string][]string{
		"a": {"b"}, "b": {"a"},
		"d": {"a"}, // depends on the cycle
		"e": {"d"}, // depends on a dependent
	}
	require.Equal(t, []string{"a", "b"}, CycleMembers(order, deps))
}

func TestCycleMembers_DependencyOfCycleNotReported(t *testing.T) {
	order := []string{"a", "b", "c"}
	deps := map[string][]string{"a": {"b", "c"}, "b": {"a"}}
	require.Equal(t, []string{"a", "b"}, CycleMembers(order, deps))
}

func TestCycleMembers_UnknownNamesIgnored(t *testing.T) {
	order := []string{"a", "b"}
	deps := map[string][]string{"a": {"ghost"}, "b": {"ghost", "a"}, "ghost": {"a"}}
	require.Nil(t, CycleMembers(order, deps))

	// A cycle that only closes through an unknown name is not a cycle.
	order = []string{"a", "b"}
	deps = map[string][]string{"a": {"ghost"}, "ghost": {"a"}, "b": {"a"}}
	require.Nil(t, CycleMembers(order, deps))

	// A real cycle is still found alongside unknown deps.
	deps = map[string][]string{"a": {"ghost", "b"}, "b": {"a", "ghost"}}
	require.Equal(t, []string{"a", "b"}, CycleMembers(order, deps))
}

func TestTopoOrder_EmptyInputs(t *testing.T) {
	require.Empty(t, TopoOrder(nil, nil))
	require.Empty(t, TopoOrder([]string{}, map[string][]string{}))
}

func TestTopoOrder_ChainListedInReverse(t *testing.T) {
	order := []string{"c", "b", "a"}
	deps := map[string][]string{"c": {"b"}, "b": {"a"}}
	require.Equal(t, []string{"a", "b", "c"}, TopoOrder(order, deps))
}

func TestTopoOrder_Diamond(t *testing.T) {
	order := []string{"d", "c", "b", "a"}
	deps := map[string][]string{"d": {"b", "c"}, "b": {"a"}, "c": {"a"}}
	require.Equal(t, []string{"a", "c", "b", "d"}, TopoOrder(order, deps))
}

func TestTopoOrder_DisjointGraphs(t *testing.T) {
	order := []string{"b2", "a2", "b1", "a1"}
	deps := map[string][]string{"b2": {"b1"}, "a2": {"a1"}}
	// b1 is the earliest ready node after b2/a2 are blocked; then b2 is ready
	// and earlier in list order than the rest.
	require.Equal(t, []string{"b1", "b2", "a1", "a2"}, TopoOrder(order, deps))
}

func TestTopoOrder_TiesKeepListOrder(t *testing.T) {
	require.Equal(t, []string{"b", "a"}, TopoOrder([]string{"b", "a"}, nil))
	require.Equal(t, []string{"a", "b"}, TopoOrder([]string{"b", "a"}, map[string][]string{"b": {"a"}}))
	require.Equal(t, []string{"c", "a", "b"}, TopoOrder([]string{"c", "a", "b"}, map[string][]string{}))
}

func TestTopoOrder_ReadyNodePrecedesEarlierBlockedNode(t *testing.T) {
	order := []string{"a", "b", "c"}
	deps := map[string][]string{"a": {"c"}}
	require.Equal(t, []string{"b", "c", "a"}, TopoOrder(order, deps))
}

func TestTopoOrder_UnknownNamesIgnored(t *testing.T) {
	order := []string{"b", "a"}
	deps := map[string][]string{"b": {"ghost", "a"}, "a": {"phantom"}, "ghost": {"b"}}
	require.Equal(t, []string{"a", "b"}, TopoOrder(order, deps))

	require.Equal(t, []string{"b", "a"}, TopoOrder(order, map[string][]string{"b": {"ghost"}}))
}

func TestTopoOrder_DuplicateDepsEntries(t *testing.T) {
	order := []string{"b", "a"}
	deps := map[string][]string{"b": {"a", "a", "a"}}
	require.Equal(t, []string{"a", "b"}, TopoOrder(order, deps))
}

func TestTopoOrder_CycleAndDependentsAppendedInOrder(t *testing.T) {
	order := []string{"x", "c1", "dep", "ok2", "c2", "ok1"}
	deps := map[string][]string{
		"c1":  {"c2"},
		"c2":  {"c1"},
		"dep": {"c1"},
		"x":   {"ok1"},
		"ok2": {"ok1"},
	}
	got := TopoOrder(order, deps)
	require.Equal(t, []string{"ok1", "x", "ok2", "c1", "dep", "c2"}, got)
}

func TestTopoOrder_SelfLoopAppended(t *testing.T) {
	order := []string{"s", "a"}
	deps := map[string][]string{"s": {"s"}}
	require.Equal(t, []string{"a", "s"}, TopoOrder(order, deps))
}

func TestTopoOrder_EveryNodeExactlyOnce(t *testing.T) {
	order := []string{"a", "b", "c", "d", "e"}
	deps := map[string][]string{"a": {"b", "b"}, "b": {"a"}, "c": {"a", "e"}, "d": {"zzz"}}
	got := TopoOrder(order, deps)
	require.ElementsMatch(t, order, got)
	require.Len(t, got, len(order))
}

func TestReaches_Direct(t *testing.T) {
	deps := map[string][]string{"a": {"b"}}
	require.True(t, Reaches(deps, "a", "b"))
	require.False(t, Reaches(deps, "b", "a"), "a dependency is not reached backwards")
}

func TestReaches_Transitive(t *testing.T) {
	deps := map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"d"}}
	require.True(t, Reaches(deps, "a", "c"))
	require.True(t, Reaches(deps, "a", "d"))
	require.True(t, Reaches(deps, "b", "d"))
	require.False(t, Reaches(deps, "d", "a"))
}

func TestReaches_Unrelated(t *testing.T) {
	deps := map[string][]string{"a": {"b"}, "c": {"d"}}
	require.False(t, Reaches(deps, "a", "c"))
	require.False(t, Reaches(deps, "a", "d"))
	require.False(t, Reaches(deps, "c", "b"))
}

func TestReaches_Self(t *testing.T) {
	deps := map[string][]string{"a": {"b"}}
	require.False(t, Reaches(deps, "a", "a"), "a node without a cycle does not reach itself")

	require.True(t, Reaches(map[string][]string{"a": {"a"}}, "a", "a"), "a self loop reaches itself")
	require.True(t, Reaches(map[string][]string{"a": {"b"}, "b": {"a"}}, "a", "a"),
		"a cycle leading back reaches itself")
}

func TestReaches_Unknown(t *testing.T) {
	deps := map[string][]string{"a": {"b"}}
	require.False(t, Reaches(deps, "ghost", "a"), "a name absent from deps has no edges")
	require.False(t, Reaches(deps, "a", "ghost"))
	require.False(t, Reaches(nil, "a", "b"))
	require.True(t, Reaches(map[string][]string{"a": {"ghost"}}, "a", "ghost"),
		"an edge to an undeclared name still reaches it")
}

func TestReaches_CycleTerminates(t *testing.T) {
	deps := map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}, "x": {"y"}}
	require.False(t, Reaches(deps, "a", "x"), "a cycle not leading to the target terminates false")
	require.True(t, Reaches(deps, "b", "a"))
	require.True(t, Reaches(deps, "c", "b"))
}
