package export

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/refliqx/relaydb/engine/internal/driver"
	"github.com/refliqx/relaydb/engine/internal/introspect"
	"github.com/refliqx/relaydb/engine/internal/protocol"
)

// PlanInput gathers everything needed to build an export plan.
type PlanInput struct {
	Conn      *driver.Conn
	Snapshot  *protocol.SchemaSnapshot
	Selection []protocol.TableSelection
	Masking   []protocol.MaskRule
	Pruning   []protocol.PruneRule
	Options   protocol.ExportOptions
}

// BuildPlan is a pure function (no DB I/O): it takes the schema and the user's
// intent and produces an FK-safe sequence of steps together with their SELECT
// SQL. Because it is pure, the plan can be shown as a preview in the UI before
// it runs.
func BuildPlan(in PlanInput) (*protocol.ExportPlan, error) {
	if in.Conn == nil || in.Snapshot == nil {
		return nil, fmt.Errorf("plan: schema snapshot is not available")
	}
	opts := withDefaults(in.Options)

	// 1. Determine the participating tables.
	included := map[string]bool{}
	if len(in.Selection) == 0 {
		for _, t := range in.Snapshot.Tables {
			if t.Kind == "table" {
				included[introspect.TableKey(t.Schema, t.Name)] = true
			}
		}
	} else {
		for _, s := range in.Selection {
			if s.Include {
				included[introspect.TableKey(s.Schema, s.Table)] = true
			}
		}
	}
	if len(included) == 0 {
		return nil, fmt.Errorf("no tables were selected for export")
	}

	// 2. Graph + topology for the selected tables only. Pruning the graph first
	//    prevents dangling edges that would make the topological sort wrong.
	graph := introspect.BuildGraph(in.Snapshot, func(schema, table string) bool {
		return included[introspect.TableKey(schema, table)]
	})
	order := introspect.TopoSort(graph, in.Snapshot.Tables)

	warns := []string{}
	if len(order.Cycles) > 0 {
		warns = append(warns, fmt.Sprintf(
			"%d FK cycles detected. Those tables are exported last and their order cannot be guaranteed without disabling FKs on import (enable `fkGuard`).",
			len(order.Cycles)))
	}

	// 3. Index the user's rules.
	maskByColumn := map[string]protocol.MaskRule{}
	for _, r := range in.Masking {
		if r.Enabled != nil && !*r.Enabled {
			continue
		}
		maskByColumn[key(r.Table, r.Column)] = r
	}
	pruneByTable := map[string]protocol.PruneRule{}
	for _, p := range in.Pruning {
		pruneByTable[introspect.TableKey(pruneSchema(p, in.Snapshot), p.Table)] = p
	}

	// 4. Build the steps following the topological order.
	byTable := map[string]protocol.Table{}
	for _, t := range in.Snapshot.Tables {
		byTable[introspect.TableKey(t.Schema, t.Name)] = t
	}

	plan := &protocol.ExportPlan{
		PlanID:     uuid.NewString(),
		Dialect:    in.Conn.Dialect.Name,
		Seed:       opts.Seed,
		Cycles:     order.Cycles,
		Warnings:   warns,
		OutputPath: opts.OutputPath,
		CreatedAt:  time.Now().UTC(),
	}

	parentsOf := map[string][]string{}
	for _, e := range graph.Edges {
		if e.From == e.To {
			continue
		}
		parentsOf[e.From] = appendUnique(parentsOf[e.From], e.To)
	}

	rowsTotal := int64(0)
	for i, tableKey := range order.Order {
		table, ok := byTable[tableKey]
		if !ok {
			continue
		}
		step := protocol.ExportStep{
			Order:       i,
			Schema:      table.Schema,
			Table:       table.Name,
			RowEstimate: table.EstimatedRows,
			DependsOn:   parentsOf[tableKey],
		}

		// Generated columns must not be part of the INSERT.
		var cols []string
		for _, c := range table.Columns {
			if c.IsGenerated {
				continue
			}
			cols = append(cols, c.Name)
			rule, masked := maskByColumn[key(table.Name, c.Name)]
			if masked {
				step.MaskedColumns = append(step.MaskedColumns, rule)
			} else if c.PiiGuess != "" {
				warns = append(warns, fmt.Sprintf(
					"column %s.%s detected as %s (%.0f%%) but has no masking rule",
					table.Name, c.Name, c.PiiGuess, c.PiiConfidence*100))
			}
		}
		if len(step.MaskedColumns) == 0 {
			step.MaskedColumns = []protocol.MaskRule{}
		}
		if len(cols) == 0 {
			continue
		}
		step.Columns = cols

		if pr, ok := pruneByTable[tableKey]; ok {
			applied := pr
			step.Prune = &applied
			if pr.Limit != nil {
				if table.EstimatedRows > int64(*pr.Limit) {
					step.RowEstimate = int64(*pr.Limit)
				}
			}
		}
		if step.RowEstimate > 0 {
			rowsTotal += step.RowEstimate
		}

		step.SelectSQL = buildSelect(in.Conn.Dialect, table, cols, step.Prune, opts)
		plan.Steps = append(plan.Steps, step)
	}

	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("no valid tables to export")
	}

	// 5. Dangerous-pruning warning: pruning a parent table leaves its children
	//    without a reference.
	for _, step := range plan.Steps {
		if step.Prune == nil || step.Prune.Limit == nil {
			continue
		}
		keyOf := introspect.TableKey(step.Schema, step.Table)
		var kids []string
		for _, e := range graph.Edges {
			if e.To == keyOf && e.From != keyOf {
				kids = appendUnique(kids, e.From)
			}
		}
		if len(kids) > 0 {
			warns = append(warns, fmt.Sprintf(
				"table %s is pruned but is referenced by %s; child rows may dangle",
				step.Table, strings.Join(kids, ", ")))
		}
	}
	plan.Warnings = warns
	return plan, nil
}

// buildSelect builds a streaming-friendly SELECT.
//
// Open questions already answered here:
//   - Without ORDER BY, row order differs between RDBMSs -> the seed file
//     changes on every run. We always ORDER BY the primary key (available from
//     introspection) so the output is reproducible.
//   - LIMIT only applies when the user prunes; 0/NULL means the whole table.
func buildSelect(d driver.Dialect, table protocol.Table, cols []string, prune *protocol.PruneRule, opts protocol.ExportOptions) string {
	quoted := make([]string, 0, len(cols))
	for _, c := range cols {
		quoted = append(quoted, d.QuoteIdent(c))
	}
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(" FROM ")
	b.WriteString(d.TableRef(table.Schema, table.Name))

	if prune != nil && strings.TrimSpace(prune.Where) != "" {
		b.WriteString(" WHERE ")
		b.WriteString(prune.Where)
	}

	if prune != nil && len(prune.OrderBy) > 0 {
		terms := make([]string, 0, len(prune.OrderBy))
		for _, o := range prune.OrderBy {
			dir := strings.ToUpper(o.Direction)
			if dir != "DESC" {
				dir = "ASC"
			}
			terms = append(terms, d.QuoteIdent(o.Column)+" "+dir)
		}
		b.WriteString(" ORDER BY ")
		b.WriteString(strings.Join(terms, ", "))
	} else if len(table.PrimaryKey) > 0 {
		terms := make([]string, 0, len(table.PrimaryKey))
		for _, pk := range table.PrimaryKey {
			terms = append(terms, d.QuoteIdent(pk)+" ASC")
		}
		b.WriteString(" ORDER BY ")
		b.WriteString(strings.Join(terms, ", "))
	}

	if prune != nil && prune.Limit != nil && *prune.Limit > 0 {
		fmt.Fprintf(&b, " LIMIT %d", *prune.Limit)
	}
	return b.String()
}

func withDefaults(o protocol.ExportOptions) protocol.ExportOptions {
	if o.BatchSize <= 0 {
		// 100 rows/INSERT: the file stays human-readable (not one giant INSERT)
		// yet is far smaller than one row per statement.
		o.BatchSize = 100
	}
	if o.FetchSize <= 0 {
		o.FetchSize = 1000
	}
	if o.Seed == "" {
		o.Seed = "relaydb-default-seed"
	}
	if o.ProgressEveryN <= 0 {
		o.ProgressEveryN = 500
	}
	return o
}

func key(table, column string) string {
	return strings.ToLower(table) + "." + strings.ToLower(column)
}

func pruneSchema(p protocol.PruneRule, snap *protocol.SchemaSnapshot) string {
	for _, t := range snap.Tables {
		if t.Name == p.Table {
			return t.Schema
		}
	}
	return ""
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// SortRules produces a stable rule order, so a saved/diffed preset does not
// "move lines" every time the UI saves.
func SortRules(rules []protocol.MaskRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if !strings.EqualFold(rules[i].Table, rules[j].Table) {
			return strings.ToLower(rules[i].Table) < strings.ToLower(rules[j].Table)
		}
		return strings.ToLower(rules[i].Column) < strings.ToLower(rules[j].Column)
	})
}
