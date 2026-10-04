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

// CycleMembers returns every node that sits on a dependency cycle — a node
// depending on itself, or one of a group of nodes that depend on each other —
// in order's order; nil when the graph is acyclic. A node that merely depends
// on a cycle is not on it. Dependencies on names not in order are ignored.
func CycleMembers(order []string, deps map[string][]string) []string {
	known := make(map[string]bool, len(order))
	for _, n := range order {
		known[n] = true
	}

	// Tarjan's strongly connected components: a component of more than one
	// node, or a single node with a self-loop, is a cycle.
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	next := 0
	onCycle := map[string]bool{}

	var connect func(n string)
	connect = func(n string) {
		index[n], low[n] = next, next
		next++
		stack = append(stack, n)
		onStack[n] = true
		for _, dep := range deps[n] {
			if !known[dep] {
				continue
			}
			if _, seen := index[dep]; !seen {
				connect(dep)
				low[n] = min(low[n], low[dep])
			} else if onStack[dep] {
				low[n] = min(low[n], index[dep])
			}
		}
		if low[n] != index[n] {
			return
		}
		var component []string
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			component = append(component, top)
			if top == n {
				break
			}
		}
		if len(component) > 1 || dependsOn(deps, n, n) {
			for _, m := range component {
				onCycle[m] = true
			}
		}
	}

	for _, n := range order {
		if _, seen := index[n]; !seen {
			connect(n)
		}
	}

	var members []string
	for _, n := range order {
		if onCycle[n] {
			members = append(members, n)
		}
	}
	return members
}

// TopoOrder returns the nodes of order so that every node comes after the
// nodes it depends on. It is stable: whenever several nodes are ready, the
// one earliest in order goes first, so ties keep their declared order.
// Dependencies on names not in order are ignored. Nodes that can never become
// ready — those on a cycle, and those depending on one — are appended at the
// end in order's order, so every node is returned exactly once.
func TopoOrder(order []string, deps map[string][]string) []string {
	position := make(map[string]int, len(order))
	for i, n := range order {
		if _, dup := position[n]; !dup {
			position[n] = i
		}
	}

	waiting := make(map[string]map[string]bool, len(order))
	dependents := map[string][]string{}
	for n := range position {
		waiting[n] = map[string]bool{}
		for _, dep := range deps[n] {
			if _, ok := position[dep]; ok && !waiting[n][dep] {
				waiting[n][dep] = true
				dependents[dep] = append(dependents[dep], n)
			}
		}
	}

	placed := make(map[string]bool, len(order))
	result := make([]string, 0, len(position))
	for {
		// The earliest-declared node with nothing left to wait for.
		pick := ""
		for _, n := range order {
			if !placed[n] && len(waiting[n]) == 0 {
				pick = n
				break
			}
		}
		if pick == "" {
			break
		}
		placed[pick] = true
		result = append(result, pick)
		for _, d := range dependents[pick] {
			delete(waiting[d], pick)
		}
	}
	for _, n := range order {
		if !placed[n] {
			placed[n] = true
			result = append(result, n)
		}
	}
	return result
}

func dependsOn(deps map[string][]string, n, target string) bool {
	for _, d := range deps[n] {
		if d == target {
			return true
		}
	}
	return false
}
