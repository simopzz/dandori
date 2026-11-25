package kitchen

import (
	"context"
	"net/http"
	"net/url"
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
	// Back is where it lands a browser without htmx.
	Back Back
}

// postedCountForm is the Count form posted in `r` for `ing`, as typed, with why each
// field was refused, by `problems`.
func postedCountForm(r *http.Request, ing domain.Ingredient, problems map[string]domain.Problem) CountForm {
	ctx := r.Context()
	f := CountForm{
		Ingredient: &ing, Location: domain.StorageLocation(r.PostFormValue("location")),
		Stated: strings.TrimSpace(r.PostFormValue("stated")), Note: r.PostFormValue("note"),
		Errs: map[string]string{}, Back: backOf(r.PostForm),
	}
	if _, refused := problems["location"]; refused {
		f.Errs["location"], f.Location = web.T(ctx, msgChooseWhere), ing.Usual
	}
	if problem, refused := problems["stated"]; refused {
		f.Errs["stated"] = web.T(ctx, web.ProblemMessage(problem, problemMessages))
	}
	return f
}

// Back is where a Stock Item's Count or move lands a browser without htmx once saved.
type Back string

const (
	// BackToKitchen lands it where the Kitchen page's own Count and move do.
	BackToKitchen Back = ""
	// BackToToday lands it on Today.
	BackToToday Back = "/"
)

// backOf is the Back the form `values` came with: BackToToday when they ask for it, and
// BackToKitchen whatever else they say, so no other page can be landed on.
func backOf(values url.Values) Back {
	if Back(values.Get("back")) == BackToToday {
		return BackToToday
	}
	return BackToKitchen
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
	// Back is where it lands a browser without htmx.
	Back Back
}

// postedMoveForm is the form posted in `r` that moves `ing` from a Storage Location,
// which held `held`, as typed, with why each field was refused, by `problems`.
func postedMoveForm(r *http.Request, ing domain.Ingredient, from domain.StorageLocation, held map[domain.StorageLocation]domain.Quantity, problems map[string]domain.Problem) MoveForm {
	ctx := r.Context()
	f := MoveForm{
		Ingredient: ing, From: from, Believed: held, To: domain.StorageLocation(r.PostFormValue("to")),
		Quantity: strings.TrimSpace(r.PostFormValue("quantity")), Note: r.PostFormValue("note"),
		Back: backOf(r.PostForm),
	}
	f.Errs = moveErrs(ctx, problems, web.Tf(ctx, msgMoreThanHeld, web.Quantity(ctx, held[from], ing.Unit)), moveProblemMessages)
	if f.Errs["to"] != "" {
		f.To = ""
	}
	return f
}

// moveErrs is why each field of a move was refused, by `problems`, in the User's
// Language: `moreThanHeld` if it was more than is held, and `messages` for its quantity's
// other `problems`.
func moveErrs(ctx context.Context, problems map[string]domain.Problem, moreThanHeld string, messages map[domain.Problem]web.Message) map[string]string {
	errs := map[string]string{}
	if _, refused := problems["to"]; refused {
		errs["to"] = web.T(ctx, msgChooseTarget)
	}
	switch problem, refused := problems["quantity"]; {
	case problem == domain.MoreThanHeld:
		errs["quantity"] = moreThanHeld
	case refused:
		errs["quantity"] = web.T(ctx, web.ProblemMessage(problem, messages))
	}
	return errs
}

// PortionLotForm is the form that counts or moves a Portion Lot, as shown: Portion is one
// of them, which names the Lot, and Typed, To and Note are as typed, with Errs why each
// field was refused, in the User's Language.
type PortionLotForm struct {
	Portion         int64
	Lot             domain.PortionLot
	Typed, To, Note string
	Errs            map[string]string
}

// postedLotForm is the form posted in `r` for `lot`, as typed in `field`.
func postedLotForm(r *http.Request, portion int64, lot domain.PortionLot, field string) PortionLotForm {
	return PortionLotForm{
		Portion: portion, Lot: lot, Typed: strings.TrimSpace(r.PostFormValue(field)),
		To: r.PostFormValue("to"), Note: r.PostFormValue("note"), Errs: map[string]string{},
	}
}

// PortionsForm is the Add Portions form as shown: what was typed, the nutritional values
// by field, and why each field was refused, in the User's Language.
type PortionsForm struct {
	Name, HowMany, Note string
	Location            domain.StorageLocation
	Values, Errs        map[string]string
}

// postedPortions is the Add Portions form posted in `r`, as typed.
func postedPortions(r *http.Request) domain.TypedPortions {
	return domain.TypedPortions{
		Name: strings.TrimSpace(r.PostFormValue("name")), HowMany: strings.TrimSpace(r.PostFormValue("how_many")),
		Note: strings.TrimSpace(r.PostFormValue("note")), Location: domain.StorageLocation(r.PostFormValue("location")),
		Nutrition: ingredients.PostedNutrition(r),
	}
}

// refusedPortionsForm is the Add Portions form as `typed`, with why each field was
// refused, by `problems`.
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

// refusals are the fields `f` refused, in the order the form shows them, for the Add
// Portions page's ErrorSummary.
func (f PortionsForm) refusals(ctx context.Context) []web.Refusal {
	var refusals []web.Refusal
	refuse := func(id string, label web.Message) {
		if err := f.Errs[id]; err != "" {
			refusals = append(refusals, web.Refusal{ID: id, Label: web.T(ctx, label), Err: err})
		}
	}
	refuse("name", msgPortionName)
	refuse("how_many", msgHowMany)
	refuse("location", msgPortionWhere)
	return append(refusals, ingredients.Refusals(ctx, func(field string) string { return field }, f.Errs)...)
}
