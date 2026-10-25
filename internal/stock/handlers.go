package stock

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve the Stock page.
type Handlers struct {
	Service *Service
	Log     *slog.Logger
}

// Show shows the Stock page.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	if v := r.URL.Query().Get("variant"); v != "" { // PROTOTYPE (design system)
		h.showPrototype(w, r, v)
		return
	}
	h.render(w, r, rejection{}, http.StatusOK)
}

// rejection is a rejected submission the Stock page is shown back with:
// the add form, a Stock Item's change form or a Portion's move.
type rejection struct {
	add ItemForm
	// changedItem is the id of the Stock Item whose change was rejected,
	// and change its form as typed.
	changedItem int64
	change      ItemForm
	// movedPortion is the id of the Portion whose move was rejected:
	// moveTo is the Storage Location asked for, moveError why it can't be.
	movedPortion int64
	moveTo       string
	moveError    string
}

func (h Handlers) render(w http.ResponseWriter, r *http.Request, rej rejection, status int) {
	ingredients, err := h.Service.Ingredients(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load ingredients", err)
		return
	}
	items, err := h.Service.Items(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load stock items", err)
		return
	}
	portions, err := h.Service.Portions(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load portions", err)
		return
	}
	p := StockPage{Add: rej.add}
	// A rejected change or move of something gone has nothing to show back.
	found := rej.changedItem == 0 && rej.movedPortion == 0
	for _, ing := range ingredients {
		p.Ingredients = append(p.Ingredients, IngredientOption{Name: ing.Name, Unit: string(ing.Unit)})
	}
	for _, it := range items {
		row := ItemRow{
			ID:         it.ID,
			Ingredient: it.Ingredient.Name,
			Quantity:   formatAmount(it.Quantity) + " " + string(it.Ingredient.Unit),
			Location:   string(it.Location),
			Form:       ItemForm{Values: itemValues(it)},
		}
		if it.ExpiresOn != nil {
			row.ExpiresOn = it.ExpiresOn.Format(dateLayout)
		}
		if it.ID == rej.changedItem {
			row.Form, found = rej.change, true
		}
		p.Items = append(p.Items, row)
	}
	for _, portion := range portions {
		row := PortionRow{ID: portion.ID, Recipe: portion.Recipe, Location: string(portion.Location)}
		if row.Recipe == "" {
			row.Recipe = "Ingredients as they are"
		}
		if portion.ID == rej.movedPortion {
			row.MoveTo, row.Error, found = rej.moveTo, rej.moveError, true
		}
		p.Portions = append(p.Portions, row)
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	templ.Handler(Page(p), templ.WithStatus(status)).ServeHTTP(w, r)
}

// Add records a Stock Item.
func (h Handlers) Add(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	item, problems := readStockItem(r.PostForm)
	if len(problems) == 0 {
		err := h.Service.Add(r.Context(), item)
		switch {
		case errors.Is(err, ErrUnknownIngredient):
			problems["ingredient"] = "Not a known Ingredient"
		case err != nil:
			httpx.ServerError(w, h.Log, "add stock item", err)
			return
		}
	}
	if len(problems) > 0 {
		h.render(w, r, rejection{add: ItemForm{Values: typed(r.PostForm), Errors: problems}}, http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/stock", http.StatusSeeOther)
}

// Update adjusts or moves a Stock Item.
func (h Handlers) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	item, problems := readStockItem(r.PostForm)
	if len(problems) > 0 {
		h.render(w, r, rejection{changedItem: id, change: ItemForm{Values: typed(r.PostForm), Errors: problems}}, http.StatusUnprocessableEntity)
		return
	}
	switch err := h.Service.Update(r.Context(), id, item); {
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "update stock item", err)
		return
	}
	http.Redirect(w, r, "/stock", http.StatusSeeOther)
}

// Remove removes a Stock Item.
func (h Handlers) Remove(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := h.Service.Remove(r.Context(), id); err != nil {
		httpx.ServerError(w, h.Log, "remove stock item", err)
		return
	}
	http.Redirect(w, r, "/stock", http.StatusSeeOther)
}

// MovePortion moves a Portion without a Slot to another Storage Location.
func (h Handlers) MovePortion(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	location := domain.StorageLocation(strings.TrimSpace(r.PostForm.Get("location")))
	var invalid *InvalidError
	switch err := h.Service.MovePortion(r.Context(), id, location); {
	case errors.As(err, &invalid):
		h.render(w, r, rejection{movedPortion: id, moveTo: string(location), moveError: invalid.Fields["Location"]}, http.StatusUnprocessableEntity)
		return
	case errors.Is(err, ErrPortionNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "move portion", err)
		return
	}
	http.Redirect(w, r, "/stock", http.StatusSeeOther)
}
