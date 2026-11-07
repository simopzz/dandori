package kitchen

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// CountForm is the Count sheet's body as shown.
type CountForm struct {
	// Query and Found are the search, shown until an Ingredient is chosen; Staple is the
	// Staple it named, if it found nothing else.
	Query  string
	Found  []domain.Ingredient
	Staple string
	// Ingredient is the one chosen, if any, and the rest its form: Believed is what
	// Dandori believes each Storage Location with Movements of it holds, Stated and Note
	// are as typed, and Errs why each field was refused, in the User's Language.
	Ingredient *domain.Ingredient
	Location   domain.StorageLocation
	Believed   map[domain.StorageLocation]domain.Quantity
	Stated     string
	Note       string
	Errs       map[string]string
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

// MoveForm is the form that moves a Stock Item, as shown.
type MoveForm struct {
	Ingredient domain.Ingredient
	From       domain.StorageLocation
	// Believed is what Dandori believes each Storage Location holds of the Ingredient;
	// To, Quantity and Note are as typed, and Errs why each field was refused, in the
	// User's Language.
	Believed map[domain.StorageLocation]domain.Quantity
	To       domain.StorageLocation
	Quantity string
	Note     string
	Errs     map[string]string
}

// parseMoveForm reads the form posted in r that moves ing from a Storage Location, as the
// quantity to move, and the form as typed with why each field is refused, if any is.
func parseMoveForm(r *http.Request, ing domain.Ingredient, from domain.StorageLocation) (domain.Quantity, MoveForm) {
	ctx := r.Context()
	to := domain.StorageLocation(r.PostFormValue("to"))
	f := MoveForm{
		Ingredient: ing, From: from, To: to,
		Quantity: strings.TrimSpace(r.PostFormValue("quantity")), Note: r.PostFormValue("note"),
		Errs: map[string]string{},
	}
	if to == from || !slices.Contains(domain.StorageLocations, to) {
		f.Errs["to"], f.To = web.T(ctx, msgChooseTarget), ""
	}
	moved, problem := domain.ParseQuantity(ing.Unit, f.Quantity)
	if problem != "" {
		f.Errs["quantity"] = web.T(ctx, web.ProblemMessage(problem, moveProblemMessages))
	}
	return moved, f
}

// refusal says why refused was refused, of unit, on the form's quantity.
func refusal(ctx context.Context, refused MoveRefused, unit domain.Unit) string {
	if refused.Problem == domain.MoreThanHeld {
		return web.Tf(ctx, msgMoreThanHeld, web.Quantity(ctx, refused.Held, unit))
	}
	return web.T(ctx, web.ProblemMessage(refused.Problem, moveProblemMessages))
}
