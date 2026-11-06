package kitchen

import (
	"net/http"
	"slices"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// CountForm is the Count sheet's body as shown.
type CountForm struct {
	// Counted is the Count just saved, if any.
	Counted *Counted
	// Query and Found are the search, shown until an Ingredient is chosen; Staple is the
	// Staple it named, if it found nothing else.
	Query  string
	Found  []domain.Ingredient
	Staple string
	// Ingredient is the one chosen, if any, and the rest its form: Believed is what
	// Dandori believes each Storage Location holds, Stated and Note are as typed, and
	// Errs why each field was refused, in the User's Language.
	Ingredient *domain.Ingredient
	Location   domain.StorageLocation
	Believed   map[domain.StorageLocation]domain.Quantity
	Stated     string
	Note       string
	Errs       map[string]string
}

// Counted is a Count just saved.
type Counted struct {
	Ingredient domain.Ingredient
	Location   domain.StorageLocation
	Stated     domain.Quantity
}

// parseCountForm reads the Count form posted in r for ing as what to count, and the form
// as typed with why each field is refused, if any is.
func parseCountForm(r *http.Request, ing domain.Ingredient) (domain.StorageLocation, domain.Quantity, CountForm) {
	ctx := r.Context()
	location := domain.StorageLocation(r.PostFormValue("location"))
	f := CountForm{
		Ingredient: &ing, Location: location,
		Stated: strings.TrimSpace(r.PostFormValue("stated")), Note: r.PostFormValue("note"),
		Errs: map[string]string{},
	}
	if !slices.Contains(domain.StorageLocations, location) {
		f.Errs["location"], f.Location = web.T(ctx, msgChooseWhere), ing.Usual
	}
	stated, problem := domain.ParseQuantity(ing.Unit, f.Stated)
	if problem != "" {
		f.Errs["stated"] = web.T(ctx, web.ProblemMessage(problem, problemMessages))
	}
	return location, stated, f
}
