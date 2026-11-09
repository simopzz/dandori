package kitchen

import (
	"context"
	"net/http"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ingredients"
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

// postedCountForm is the Count form posted in r for ing, as typed, with why each field
// was refused, by problems.
func postedCountForm(r *http.Request, ing domain.Ingredient, problems map[string]domain.Problem) CountForm {
	ctx := r.Context()
	f := CountForm{
		Ingredient: &ing, Location: domain.StorageLocation(r.PostFormValue("location")),
		Stated: strings.TrimSpace(r.PostFormValue("stated")), Note: r.PostFormValue("note"),
		Errs: map[string]string{},
	}
	if _, refused := problems["location"]; refused {
		f.Errs["location"], f.Location = web.T(ctx, msgChooseWhere), ing.Usual
	}
	if problem, refused := problems["stated"]; refused {
		f.Errs["stated"] = web.T(ctx, web.ProblemMessage(problem, problemMessages))
	}
	return f
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

// postedMoveForm is the form posted in r that moves ing from a Storage Location, which
// held held, as typed, with why each field was refused, by problems.
func postedMoveForm(r *http.Request, ing domain.Ingredient, from domain.StorageLocation, held map[domain.StorageLocation]domain.Quantity, problems map[string]domain.Problem) MoveForm {
	ctx := r.Context()
	f := MoveForm{
		Ingredient: ing, From: from, Believed: held, To: domain.StorageLocation(r.PostFormValue("to")),
		Quantity: strings.TrimSpace(r.PostFormValue("quantity")), Note: r.PostFormValue("note"),
		Errs: map[string]string{},
	}
	if _, refused := problems["to"]; refused {
		f.Errs["to"], f.To = web.T(ctx, msgChooseTarget), ""
	}
	switch problem, refused := problems["quantity"]; {
	case problem == domain.MoreThanHeld:
		f.Errs["quantity"] = web.Tf(ctx, msgMoreThanHeld, web.Quantity(ctx, held[from], ing.Unit))
	case refused:
		f.Errs["quantity"] = web.T(ctx, web.ProblemMessage(problem, moveProblemMessages))
	}
	return f
}

// PortionsForm is the Add Portions form as shown: what was typed, the nutritional values
// by field, and why each field was refused, in the User's Language.
type PortionsForm struct {
	Name, HowMany, Note string
	Location            domain.StorageLocation
	Values, Errs        map[string]string
}

// postedPortions is the Add Portions form posted in r, as typed.
func postedPortions(r *http.Request) domain.TypedPortions {
	return domain.TypedPortions{
		Name: strings.TrimSpace(r.PostFormValue("name")), HowMany: strings.TrimSpace(r.PostFormValue("how_many")),
		Note: strings.TrimSpace(r.PostFormValue("note")), Location: domain.StorageLocation(r.PostFormValue("location")),
		Nutrition: ingredients.PostedNutrition(r),
	}
}

// refusedPortionsForm is the Add Portions form as typed, with why each field was refused,
// by problems.
func refusedPortionsForm(ctx context.Context, typed domain.TypedPortions, problems map[string]domain.Problem) PortionsForm {
	f := PortionsForm{
		Name: typed.Name, HowMany: typed.HowMany, Note: typed.Note, Location: typed.Location,
		Values: typed.Nutrition, Errs: ingredients.NutritionErrs(ctx, problems),
	}
	if _, refused := problems["name"]; refused {
		f.Errs["name"] = web.T(ctx, msgPortionNameRequired)
	}
	if problem, refused := problems["how_many"]; refused {
		f.Errs["how_many"] = web.T(ctx, web.ProblemMessage(problem, howManyProblemMessages))
	}
	if _, refused := problems["location"]; refused {
		f.Errs["location"], f.Location = web.T(ctx, msgChooseWhere), ""
	}
	return f
}
