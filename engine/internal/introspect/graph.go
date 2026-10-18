package introspect

import (
	"fmt"
	"sort"

	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// TableKey is the stable table identity used by the graph.
func TableKey(schema, table string) string {
	if schema == "" || schema == "main" || schema == "public" {
		return table
	}
	return schema + "." + table
}

// BuildGraph derives nodes and edges from the schema snapshot.
//
// Edge direction: From = child (the FK owner) -> To = parent (the referenced
// table). For import, the parent MUST be inserted first.
func BuildGraph(snap *protocol.SchemaSnapshot, include func(schema, table string) bool) protocol.GraphResult {
	inc := func(s, t string) bool { return include == nil || include(s, t) }

	nodes := map[string]*protocol.GraphNode{}
	for _, t := range snap.Tables {
		if !inc(t.Schema, t.Name) {
			continue
		}
		key := TableKey(t.Schema, t.Name)
		nodes[key] = &protocol.GraphNode{ID: key, Label: t.Name, EstimatedRows: t.EstimatedRows}
	}

	nullable := map[string]bool{}
	for _, t := range snap.Tables {
		for _, c := range t.Columns {
			nullable[TableKey(t.Schema, t.Name)+"."+c.Name] = c.Nullable
		}
	}

	var edges []protocol.GraphEdge
	seen := map[string]bool{}
	for _, fk := range snap.ForeignKeys {
		from := TableKey(fk.Schema, fk.Table)
		to := TableKey(fk.RefSchema, fk.RefTable)
		fn, ok := nodes[from]
		if !ok {
			continue
		}
		tn, ok := nodes[to]
		if !ok {
			// The parent is not being exported -> the child would dangle.
			continue
		}
		// One composite constraint = one visual edge.
		sig := from + "->" + to + ":" + fk.ConstraintName
		if seen[sig] {
			continue
		}
		seen[sig] = true

		fn.OutDegree++
		tn.InDegree++
		edges = append(edges, protocol.GraphEdge{
			From:       from,
			To:         to,
			Column:     fk.Column,
			RefColumn:  fk.RefColumn,
			Constraint: fk.ConstraintName,
			Nullable:   nullable[TableKey(fk.Schema, fk.Table)+"."+fk.Column],
		})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	out := protocol.GraphResult{Edges: edges}
	keys := make([]string, 0, len(nodes))
	for k := range nodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Nodes = append(out.Nodes, *nodes[k])
	}
	out.Cycles = findCycles(keys, edges)
	return out
}

// TopoSort produces a safe insert order using Kahn's algorithm.
//
// Important details:
//   - Self-references (a table pointing at itself) are ignored for ordering;
//     their rows can still be inserted as long as the FK is nullable or filled
//     in afterwards. This is recorded as a warning.
//   - Cycles between tables cannot be resolved by ordering alone; the tables
//     involved are returned in Deferred and the UI must enable the FK guard on
//     import.
func TopoSort(g protocol.GraphResult, tables []protocol.Table) protocol.OrderResult {
	keys := make([]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		keys = append(keys, n.ID)
	}
	sort.Strings(keys)

	parents := map[string]map[string]bool{}
	children := map[string][]string{}
	for _, k := range keys {
		parents[k] = map[string]bool{}
	}
	for _, e := range g.Edges {
		if e.From == e.To {
			continue // self-loop
		}
		if !parents[e.From][e.To] {
			parents[e.From][e.To] = true
			children[e.To] = append(children[e.To], e.From)
		}
	}

	estimate := map[string]int64{}
	for _, n := range g.Nodes {
		estimate[n.ID] = n.EstimatedRows
	}

	remaining := map[string]int{}
	var queue []string
	for _, k := range keys {
		remaining[k] = len(parents[k])
		if remaining[k] == 0 {
			queue = append(queue, k)
		}
	}

	var order []string
	for len(queue) > 0 {
		// Prefer the smallest tables first within the same level: a failed
		// import reports earlier rows, giving faster feedback.
		sort.Slice(queue, func(i, j int) bool {
			ei, ej := estimate[queue[i]], estimate[queue[j]]
			if ei < 0 {
				ei = 1 << 62
			}
			if ej < 0 {
				ej = 1 << 62
			}
			if ei != ej {
				return ei < ej
			}
			return queue[i] < queue[j]
		})
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)

		for _, ch := range children[cur] {
			remaining[ch]--
			if remaining[ch] == 0 {
				queue = append(queue, ch)
			}
		}
	}

	res := protocol.OrderResult{Order: order, Cycles: g.Cycles}
	emitted := map[string]bool{}
	for _, k := range order {
		emitted[k] = true
	}
	for _, k := range keys {
		if !emitted[k] {
			res.Deferred = append(res.Deferred, k)
		}
	}
	sort.Strings(res.Deferred)
	return res
}

// findCycles returns SCCs (Strongly Connected Components) larger than 1 plus
// self-loops, using the iterative Tarjan algorithm.
func findCycles(keys []string, edges []protocol.GraphEdge) [][]string {
	adj := map[string][]string{}
	for _, k := range keys {
		adj[k] = nil
	}
	selfLoops := map[string]bool{}
	for _, e := range edges {
		if e.From == e.To {
			selfLoops[e.From] = true
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
	}

	index := 0
	idx := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var sccs [][]string

	type frame struct {
		node string
		next int
	}

	for _, root := range keys {
		if _, done := idx[root]; done {
			continue
		}
		work := []frame{{node: root}}
		idx[root], low[root] = index, index
		index++
		stack = append(stack, root)
		onStack[root] = true

		for len(work) > 0 {
			f := &work[len(work)-1]
			if f.next < len(adj[f.node]) {
				w := adj[f.node][f.next]
				f.next++
				if _, seen := idx[w]; !seen {
					idx[w], low[w] = index, index
					index++
					stack = append(stack, w)
					onStack[w] = true
					work = append(work, frame{node: w})
				} else if onStack[w] {
					if idx[w] < low[f.node] {
						low[f.node] = idx[w]
					}
				}
				continue
			}
			v := f.node
			work = work[:len(work)-1]
			if len(work) > 0 {
				p := work[len(work)-1].node
				if low[v] < low[p] {
					low[p] = low[v]
				}
			}
			if low[v] == idx[v] {
				var comp []string
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp = append(comp, w)
					if w == v {
						break
					}
				}
				if len(comp) > 1 {
					sort.Strings(comp)
					sccs = append(sccs, comp)
				}
			}
		}
	}

	for k := range selfLoops {
		sccs = append(sccs, []string{fmt.Sprintf("%s (self-reference)", k)})
	}
	sort.Slice(sccs, func(i, j int) bool { return sccs[i][0] < sccs[j][0] })
	return sccs
}
