package kitchen

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// Every Cause the domain declares is told in both Languages, on both sides of a move: one
// missing from causeNames would show a blank line in a Stock Item's Movements. The Causes
// are read from the source, so a new one can't be left out here.
func TestEveryCauseIsTold(t *testing.T) {
	causes := declaredCauses(t)
	if len(causes) == 0 {
		t.Fatal("found no Cause in ../domain/ledger.go")
	}
	for _, language := range domain.Languages {
		ctx := web.WithLanguage(context.Background(), language)
		for _, cause := range causes {
			for _, from := range domain.StorageLocations {
				for _, to := range domain.StorageLocations {
					if from == to {
						continue
					}
					m := domain.StockMovement{Movement: domain.Movement{
						Cause: cause, To: to, Key: domain.StockKey{Location: from},
					}}
					for _, location := range []domain.StorageLocation{from, to} {
						if causeText(ctx, m, location) == "" {
							t.Errorf("%s from %s to %s is told as nothing in %s, seen %s", cause, from, to, language, location)
						}
					}
				}
			}
		}
	}
}

// declaredCauses are the constants of type Cause in the domain's ledger.go.
func declaredCauses(t *testing.T) []domain.Cause {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../domain/ledger.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var causes []domain.Cause
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if typ, ok := spec.Type.(*ast.Ident); !ok || typ.Name != "Cause" {
			return true
		}
		for _, value := range spec.Values {
			lit, ok := value.(*ast.BasicLit)
			if !ok {
				t.Fatalf("a Cause isn't a string literal: %v", value)
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			causes = append(causes, domain.Cause(text))
		}
		return true
	})
	return causes
}
