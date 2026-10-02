// Package depgraph holds the graph algorithms shared by every document that
// declares dependencies between named items: plan tasks and epic specs.
package depgraph

// FindCycle returns the first dependency cycle found among the named nodes,
// as a path in dependency order with its first node repeated at the end; nil
// when the graph is acyclic. order lists every node in declaration order and
// deps maps a node to the nodes it depends on. The search visits nodes in
// order, so the report is stable for a given input. Dependencies on names not
// in order are ignored: callers report unknown names before cycles.
func FindCycle(order []string, deps map[string][]string) []string {
	const (
		white = iota
		grey
		black
	)
	known := make(map[string]bool, len(order))
	for _, n := range order {
		known[n] = true
	}
	colour := map[string]int{}
	var stack []string
	var found []string

	var visit func(n string) bool
	visit = func(n string) bool {
		colour[n] = grey
		stack = append(stack, n)
		for _, dep := range deps[n] {
			if !known[dep] {
				continue
			}
			switch colour[dep] {
			case grey:
				for i, s := range stack {
					if s == dep {
						found = append(append([]string{}, stack[i:]...), dep)
						return true
					}
				}
			case white:
				if visit(dep) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		colour[n] = black
		return false
	}

	for _, n := range order {
		if colour[n] == white && visit(n) {
			return found
		}
	}
	return nil
}
