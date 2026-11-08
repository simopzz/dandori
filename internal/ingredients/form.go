package ingredients

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// nutrients are the add form's nutritional value fields, in the order it shows them.
var nutrients = []string{"kcal", "protein", "carbs", "fat", "fibre", "sugar"}

// AddForm is the add form as shown: what was typed, and why it was refused by field in
// the User's Language.
type AddForm struct {
	Name, Unit, Usual string
	// Values are the nutritional values as typed, by field.
	Values map[string]string
	Staple bool
	Errs   map[string]string
	// Suggested is the Ingredient the name typed is close to, which the User may have
	// meant; Picked is the one they said they meant, which adds nothing.
	Suggested, Picked string
	// Counting says the User is adding it to count it, from a Count's offer.
	Counting bool
}

// EditForm is the edit form of the Ingredient ID, when it was posted and refused: the
// form as typed, and why it was refused, by field, or why the Ingredient wasn't deleted.
type EditForm struct {
	ID int64
	AddForm
	DeleteErr string
}

// editFormOf is the edit form of ing as it is, before anything is typed.
func editFormOf(ing domain.Ingredient) EditForm {
	f := EditForm{ID: ing.ID, AddForm: AddForm{
		Name: ing.Name, Unit: string(ing.Unit), Usual: string(ing.Usual), Staple: ing.Staple, Values: map[string]string{},
	}}
	n := ing.Nutrition
	for field, a := range map[string]domain.Amount{
		"kcal": n.Kcal, "protein": n.Protein, "carbs": n.Carbs, "fat": n.Fat, "fibre": n.Fibre, "sugar": n.Sugar,
	} {
		if a.Known {
			f.Values[field] = strconv.FormatFloat(a.Value, 'f', -1, 64)
		}
	}
	return f
}

// parseAddForm reads the add form posted in r as an Ingredient, and the form as typed
// with why each field is refused, if any is.
func parseAddForm(r *http.Request) (domain.Ingredient, AddForm) {
	ctx := r.Context()
	f := AddForm{
		Name:     strings.TrimSpace(r.PostFormValue("name")),
		Unit:     r.PostFormValue("unit"),
		Usual:    r.PostFormValue("usual"),
		Staple:   r.PostFormValue("staple") != "",
		Counting: r.PostFormValue("counting") != "",
	}
	var nutrition domain.Nutrition
	nutrition, f.Values, f.Errs = PostedNutrition(r)
	ing := domain.Ingredient{
		Name: f.Name, Unit: domain.Unit(f.Unit), Usual: domain.StorageLocation(f.Usual), Staple: f.Staple,
		Nutrition: nutrition,
	}
	for field, problem := range ing.Problems() {
		if _, refused := f.Errs[field]; refused {
			continue
		}
		f.Errs[field] = web.T(ctx, problemMessage(field, problem))
	}
	return ing, f
}

// PostedNutrition reads the nutritional values posted in r, with the values as typed and
// why each field is refused, by field, in the User's Language: the four always known
// are required.
func PostedNutrition(r *http.Request) (n domain.Nutrition, values, errs map[string]string) {
	ctx := r.Context()
	values, errs = map[string]string{}, map[string]string{}
	amounts := map[string]domain.Amount{}
	for _, field := range nutrients {
		values[field] = strings.TrimSpace(r.PostFormValue(field))
		a, problem := domain.ParseAmount(values[field])
		if problem != "" {
			errs[field] = web.T(ctx, web.ProblemMessage(problem, problemMessages))
		}
		amounts[field] = a
	}
	n = domain.Nutrition{
		Kcal: amounts["kcal"], Protein: amounts["protein"], Carbs: amounts["carbs"], Fat: amounts["fat"],
		Fibre: amounts["fibre"], Sugar: amounts["sugar"],
	}
	for field, problem := range n.Problems() {
		if _, refused := errs[field]; !refused {
			errs[field] = web.T(ctx, web.ProblemMessage(problem, problemMessages))
		}
	}
	return n, values, errs
}

// problemMessage says why field is refused for problem.
func problemMessage(field string, problem domain.Problem) web.Message {
	switch field {
	case "name":
		return msgNameRequired
	case "unit":
		return msgUnitRequired
	case "usual":
		return msgUsualRequired
	}
	return web.ProblemMessage(problem, problemMessages)
}
