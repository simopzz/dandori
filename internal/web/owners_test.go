package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// owners maps every table the migrations create to the feature that owns it, whose code
// alone holds its rules (ADR-0006, ADR-0019).
var owners = map[string]string{
	"user":                  "auth",
	"profile":               "profile",
	"kitchen":               "ledger",
	"movement":              "ledger",
	"portion":               "ledger",
	"movement_portion":      "ledger",
	"ingredient":            "ingredients",
	"recipe":                "recipes",
	"recipe_ingredient":     "recipes",
	"recipe_draft":          "recipes",
	"run":                   "runs",
	"trace_event":           "runs",
	"cycle":                 "planning",
	"cycle_run":             "planning",
	"plan_proposal":         "planning",
	"prep_session":          "planning",
	"planned_portion":       "planning",
	"kept_portion":          "planning",
	"planning_note":         "planning",
	"freezer_split":         "planning",
	"proposal_recipe_draft": "planning",
	"grocery_line":          "planning",
}

// storeCrossing is a store's function, as feature.function, writing or locking a table
// another feature owns.
type storeCrossing struct{ function, table string }

// cascade is a foreign key from one table to another that cascades on delete.
type cascade struct{ from, to string }

// allowedStoreCrossings are the store crossings allowed, each for the reason given.
var allowedStoreCrossings = map[storeCrossing]string{
	{"recipes.storeDraft", "run"}: "it locks the Run, so a cancel lands before or after a Recipe Draft is stored, never between; doing it through runs' API is a follow-up (#287)",
}

// allowedCascades are the cascades from one feature's table to another's allowed, each
// for the reason given.
var allowedCascades = map[cascade]string{
	{"recipe_draft", "run"}: "no code deletes a Run: only deleting a User does, and that cascades everything anyway",
	{"cycle_run", "run"}:    "no code deletes a Run: only deleting a User does, and that cascades everything anyway",
}

// table matches a table's name in SQL, quoted or not, perhaps after ONLY or a schema;
// its group is the name.
const table = `(?:ONLY\s+)?(?:"?\w+"?\.)?("?\w+"?)`

var (
	sqlComment = regexp.MustCompile(`--.*`)
	tableDDL   = regexp.MustCompile(`(?is)^\s*(CREATE|ALTER)\s+(?:(?:UNLOGGED|TEMP|TEMPORARY)\s+)?TABLE\s+(?:IF\s+(?:NOT\s+)?EXISTS\s+)?` + table)
	// foreignKey's second group is what follows the referenced columns, up to the end
	// of the constraint: where ON DELETE stands.
	foreignKey = regexp.MustCompile(`(?i)REFERENCES\s+` + table + `\s*(?:\([^)]*\))?([^,;)]*)`)
	cascades   = regexp.MustCompile(`(?i)ON\s+DELETE\s+CASCADE`)
	written    = regexp.MustCompile(`(?i)\b(?:INSERT\s+INTO|DELETE\s+FROM|UPDATE|MERGE\s+INTO)\s+` + table)
	notWritten = regexp.MustCompile(`(?i)\b(?:DO|THEN|FOR|FOR\s+NO\s+KEY)\s+UPDATE\b`)
	locks      = regexp.MustCompile(`(?i)\bFOR\s+(?:UPDATE|NO\s+KEY\s+UPDATE|SHARE|KEY\s+SHARE)\b`)
	// shortcut: readFrom takes the first table of a FROM, so FROM a, b misses b; match
	// the whole list if a locking statement ever joins with a comma.
	readFrom = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+` + table)
)

// statement is one SQL statement, where it stands and, in a store, the function holding
// it, as feature.function.
type statement struct{ where, function, sql string }

// tableName is `name` unquoted, as owners keys it.
func tableName(name string) string { return strings.ToLower(strings.Trim(name, `"`)) }

// upStatements are the statements of every migration's Up section, without comments.
func upStatements(t *testing.T) []statement {
	t.Helper()
	paths, err := filepath.Glob("../db/migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	var stmts []statement
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(b), "-- +goose Down")
		for s := range strings.SplitSeq(sqlComment.ReplaceAllString(up, ""), ";") {
			stmts = append(stmts, statement{where: path, sql: s})
		}
	}
	return stmts
}

// Every table the migrations create has an owner in owners, and owners names only tables
// they create, so a new table fails until someone decides which feature owns it.
func TestEveryTableHasAnOwningFeature(t *testing.T) {
	created := map[string]bool{}
	for _, s := range upStatements(t) {
		if m := tableDDL.FindStringSubmatch(s.sql); m != nil && strings.EqualFold(m[1], "CREATE") {
			table := tableName(m[2])
			created[table] = true
			if _, ok := owners[table]; !ok {
				t.Errorf("%s: creates %s, which owners gives no feature (ADR-0006)", s.where, table)
			}
		}
	}
	for table := range owners {
		if !created[table] {
			t.Errorf("owners names %s, which no migration creates", table)
		}
	}
}

// A foreign key from one feature's table to another's doesn't cascade on delete, so
// deleting one feature's rows never deletes another's. The keys to "user" are exempt:
// deleting a User removes everything. Which way a key points is not checked: ADR-0019
// sanctions movement's key up to ingredient.
func TestNoCascadeCrossesFeatures(t *testing.T) {
	used := map[cascade]bool{}
	for _, s := range upStatements(t) {
		m := tableDDL.FindStringSubmatch(s.sql)
		if m == nil {
			continue
		}
		from := tableName(m[2])
		for _, fk := range foreignKey.FindAllStringSubmatch(s.sql, -1) {
			to := tableName(fk[1])
			unowned := false
			for _, table := range []string{from, to} {
				if _, ok := owners[table]; !ok {
					t.Errorf("%s: a foreign key between %s and %s, but owners gives %s no feature: a table it lacks, or SQL this test misreads", s.where, from, to, table)
					unowned = true
				}
			}
			if unowned {
				continue
			}
			if to == "user" || owners[from] == owners[to] || !cascades.MatchString(fk[2]) {
				continue
			}
			c := cascade{from, to}
			if _, ok := allowedCascades[c]; ok {
				used[c] = true
				continue
			}
			t.Errorf("%s: %s's foreign key to %s cascades on delete, from %s's table to %s's (ADR-0006)",
				s.where, from, to, owners[from], owners[to])
		}
	}
	checkAllowedUsed(t, allowedCascades, used)
}

// A feature's store writes and locks only tables the feature owns: the target of an
// INSERT, UPDATE or DELETE, and every table in the FROM or JOIN of a statement locking
// rows. Reads of another feature's tables are not checked.
func TestStoresWriteAndLockOnlyTheirOwnTables(t *testing.T) {
	paths, err := filepath.Glob("../*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	used := map[storeCrossing]bool{}
	for _, path := range paths {
		feature := filepath.Base(filepath.Dir(path))
		tier, _ := fileTier(filepath.Base(path))
		if tier != tierStore || strings.HasSuffix(path, "_test.go") || slices.Contains(shared, feature) {
			continue
		}
		for _, s := range storeSQL(t, feature, path) {
			for _, table := range touched(s.sql) {
				owner, ok := owners[table]
				if !ok {
					t.Errorf("%s: %s writes or locks %s, which owners gives no feature: a table it lacks, or SQL this test misreads", s.where, s.function, table)
					continue
				}
				if owner == feature {
					continue
				}
				c := storeCrossing{s.function, table}
				if _, ok := allowedStoreCrossings[c]; ok {
					used[c] = true
					continue
				}
				t.Errorf("%s: %s writes or locks %s, which %s owns (ADR-0006)", s.where, s.function, table, owner)
			}
		}
	}
	checkAllowedUsed(t, allowedStoreCrossings, used)
}

// storeSQL is the SQL of each database call in the functions of `feature`'s Go file at
// `path`: the string literals among its arguments.
//
// shortcut: SQL held in a variable or a constant outside the call, a CopyFrom's table and
// a pgx.Batch's queries are not seen; read them if a store ever writes that way.
func storeSQL(t *testing.T, feature, path string) []statement {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var stmts []statement
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || !slices.Contains(sqlCalls, sel.Sel.Name) {
				return true
			}
			var sql strings.Builder
			for _, arg := range call.Args {
				ast.Inspect(arg, func(n ast.Node) bool {
					if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						s, _ := strconv.Unquote(lit.Value)
						sql.WriteString(s + " ")
					}
					return true
				})
			}
			stmts = append(stmts, statement{fset.Position(call.Pos()).String(), feature + "." + fn.Name.Name, sql.String()})
			return true
		})
	}
	return stmts
}

// touched are the `tables` `sql` writes, and those it reads from if it locks rows.
func touched(sql string) (tables []string) {
	for _, m := range written.FindAllStringSubmatch(notWritten.ReplaceAllString(sql, ""), -1) {
		tables = append(tables, tableName(m[1]))
	}
	if locks.MatchString(sql) {
		for _, m := range readFrom.FindAllStringSubmatch(sql, -1) {
			tables = append(tables, tableName(m[1]))
		}
	}
	return tables
}

// checkAllowedUsed fails on an allowance nothing needs any more, so the list shrinks
// once a leak is fixed.
func checkAllowedUsed[K comparable](t *testing.T, allowed map[K]string, used map[K]bool) {
	t.Helper()
	for c := range allowed {
		if !used[c] {
			t.Errorf("%+v is allowed but matches nothing any more: remove it", c)
		}
	}
}
