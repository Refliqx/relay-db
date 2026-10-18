package introspect

import (
	"reflect"
	"testing"

	"github.com/refliqx/relaydb/engine/internal/protocol"
)

func snap(tables []string, fks []protocol.ForeignKey, nullable ...string) *protocol.SchemaSnapshot {
	s := &protocol.SchemaSnapshot{Dialect: "postgres"}
	for _, name := range tables {
		s.Tables = append(s.Tables, protocol.Table{Schema: "public", Name: name, Kind: "table", EstimatedRows: 10})
	}
	isNullable := map[string]bool{}
	for _, n := range nullable {
		isNullable[n] = true
	}
	for i := range fks {
		s.ForeignKeys = append(s.ForeignKeys, fks[i])
	}
	return s
}

func fk(table, column, refTable, refColumn string) protocol.ForeignKey {
	return protocol.ForeignKey{
		ConstraintName: "fk_" + table + "_" + refTable,
		Schema:         "public", Table: table, Column: column,
		RefSchema: "public", RefTable: refTable, RefColumn: refColumn,
	}
}

func TestTopoSortParentsFirst(t *testing.T) {
	// users <- posts <- comments  (child points at parent)
	s := snap([]string{"comments", "posts", "users", "tags"}, []protocol.ForeignKey{
		fk("posts", "user_id", "users", "id"),
		fk("comments", "post_id", "posts", "id"),
		fk("comments", "user_id", "users", "id"),
	})
	g := BuildGraph(s, nil)
	order := TopoSort(g, s.Tables)

	pos := map[string]int{}
	for i, id := range order.Order {
		pos[id] = i
	}
	if pos["users"] > pos["posts"] {
		t.Fatalf("users must come before posts: %v", order.Order)
	}
	if pos["posts"] > pos["comments"] {
		t.Fatalf("posts must come before comments: %v", order.Order)
	}
	if len(order.Deferred) != 0 {
		t.Fatalf("there are no cycles, so deferred must be empty: %v", order.Deferred)
	}
	if len(order.Order) != 4 {
		t.Fatalf("every table must appear in the order: %v", order.Order)
	}
}

func TestCycleDetectedAndDeferred(t *testing.T) {
	// a -> b -> a (two-way cycle), c stands alone.
	s := snap([]string{"a", "b", "c"}, []protocol.ForeignKey{
		fk("a", "b_id", "b", "id"),
		fk("b", "a_id", "a", "id"),
	})
	g := BuildGraph(s, nil)
	order := TopoSort(g, s.Tables)

	if len(g.Cycles) == 0 {
		t.Fatalf("the cycle must be detected")
	}
	// Tables outside the cycle can still be ordered.
	found := false
	for _, id := range order.Order {
		if id == "c" {
			found = true
		}
	}
	if !found {
		t.Fatalf("table c must still be exported: %v", order.Order)
	}
	if len(order.Deferred) != 2 {
		t.Fatalf("a and b must be deferred: %v", order.Deferred)
	}
}

func TestSelfReferenceDoesNotBlock(t *testing.T) {
	s := snap([]string{"categories"}, []protocol.ForeignKey{
		fk("categories", "parent_id", "categories", "id"),
	})
	g := BuildGraph(s, nil)
	order := TopoSort(g, s.Tables)
	if len(order.Order) != 1 || order.Order[0] != "categories" {
		t.Fatalf("a self-reference must not lock the table: %v", order.Order)
	}
	if len(g.Cycles) == 0 {
		t.Fatalf("a self-reference must be reported as a cycle note")
	}
}

func TestBuildGraphDirectionAndDegrees(t *testing.T) {
	s := snap([]string{"users", "posts"}, []protocol.ForeignKey{fk("posts", "user_id", "users", "id")})
	g := BuildGraph(s, nil)

	if len(g.Edges) != 1 {
		t.Fatalf("there must be 1 edge: %v", g.Edges)
	}
	e := g.Edges[0]
	if e.From != "posts" || e.To != "users" {
		t.Fatalf("wrong edge direction (must be child->parent): %+v", e)
	}
	for _, n := range g.Nodes {
		switch n.ID {
		case "posts":
			if n.OutDegree != 1 || n.InDegree != 0 {
				t.Fatalf("wrong degree for posts: %+v", n)
			}
		case "users":
			if n.InDegree != 1 || n.OutDegree != 0 {
				t.Fatalf("wrong degree for users: %+v", n)
			}
		}
	}
}

func TestGraphExcludesTablesNotSelected(t *testing.T) {
	// Selecting only `posts` -> the edge to `users` must be dropped, not left
	// dangling.
	s := snap([]string{"users", "posts"}, []protocol.ForeignKey{fk("posts", "user_id", "users", "id")})
	g := BuildGraph(s, func(schema, table string) bool { return table == "posts" })
	if len(g.Edges) != 0 {
		t.Fatalf("an edge to a table that is not exported must be ignored: %v", g.Edges)
	}
	order := TopoSort(g, s.Tables)
	if !reflect.DeepEqual(order.Order, []string{"posts"}) {
		t.Fatalf("the order must contain only posts: %v", order.Order)
	}
}

func TestGuessPII(t *testing.T) {
	cases := []struct {
		column string
		kind   string
		want   string
		empty  bool
	}{
		{column: "email", kind: "varchar", want: "email"},
		{column: "user_email_address", kind: "text", want: "email"},
		{column: "password_hash", kind: "varchar", want: "password"},
		{column: "no_hp", kind: "varchar", want: "phone"},
		{column: "full_name", kind: "varchar", want: "person_name"},
		{column: "nik", kind: "char", want: "national_id"},
		{column: "created_at", kind: "timestamp", empty: true},
		{column: "quantity", kind: "integer", empty: true},
	}
	for _, c := range cases {
		got, conf := GuessPII(c.column, c.kind)
		if c.empty {
			if got != "" {
				t.Errorf("%s: should not be detected, got %q", c.column, got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("%s: want %q, got %q", c.column, c.want, got)
		}
		if conf <= 0 || conf > 1 {
			t.Errorf("%s: confidence out of range: %v", c.column, conf)
		}
	}
}

func TestSuggestStrategyCoversCategories(t *testing.T) {
	pairs := map[string]string{
		"email":       protocol.StrategyFakeEmail,
		"password":    protocol.StrategyDefaultHash,
		"phone":       protocol.StrategyFakePhone,
		"person_name": protocol.StrategyFakeName,
		"national_id": protocol.StrategyRedact,
		"ip":          protocol.StrategyNullify,
	}
	for category, want := range pairs {
		if got := SuggestStrategy(category); got != want {
			t.Errorf("category %s: want %s, got %s", category, want, got)
		}
	}
}
