package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve the Known Catalog page.
type Handlers struct {
	Service *Service
	Log     *slog.Logger
}

// Show shows the Known Catalog page: the seed, every Catalog Product and
// every Ingredient's Product Mapping.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, nil, http.StatusOK)
}

// render shows the Known Catalog page, with rejected marking the mapping
// whose change was rejected.
func (h Handlers) render(w http.ResponseWriter, r *http.Request, rejected *MappingRow, status int) {
	p, err := h.page(r.Context(), rejected)
	if err != nil {
		httpx.ServerError(w, h.Log, "load known catalog", err)
		return
	}
	templ.Handler(CatalogPage(p), templ.WithStatus(status)).ServeHTTP(w, r)
}

// renderMapping answers htmx with the row of the Ingredient with id alone,
// so the page keeps its place (ADR 0009); rejected marks a rejected change.
// A row that is gone is answered with nothing, which takes it away.
func (h Handlers) renderMapping(w http.ResponseWriter, r *http.Request, id int64, rejected *MappingRow, status int) {
	p, err := h.page(r.Context(), rejected)
	if err != nil {
		httpx.ServerError(w, h.Log, "load known catalog", err)
		return
	}
	for _, row := range p.Mappings {
		if row.IngredientID == id {
			templ.Handler(mappingRow(row), templ.WithStatus(status)).ServeHTTP(w, r)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// page is the Known Catalog page's content, with rejected marking the
// mapping whose change was rejected.
func (h Handlers) page(ctx context.Context, rejected *MappingRow) (Page, error) {
	products, err := h.Service.Products(ctx)
	if err != nil {
		return Page{}, err
	}
	mappings, err := h.Service.Mappings(ctx)
	if err != nil {
		return Page{}, err
	}
	p := Page{Orders: h.Service.Orders()}
	for _, prod := range products {
		p.Products = append(p.Products, newProductRow(prod.CatalogProduct))
	}
	for _, m := range mappings {
		row := MappingRow{
			IngredientID: m.IngredientID,
			Ingredient:   m.Ingredient.Name,
			Unit:         string(m.Ingredient.Unit),
			ProductID:    m.ProductID,
			Product:      m.ProductName,
			Proposed:     m.Proposed,
		}
		for _, prod := range products {
			if domain.MappingProblem(m.Ingredient, prod.CatalogProduct) == "" {
				row.Options = append(row.Options, Option{ID: prod.ID, Name: prod.Name})
			}
		}
		if rejected != nil && rejected.IngredientID == m.IngredientID {
			row.Selected, row.Error = rejected.Selected, rejected.Error
		} else {
			row.Selected = m.ProductID
		}
		if m.ProductID == 0 && len(products) > 0 {
			p.CanPropose = true
		}
		p.Mappings = append(p.Mappings, row)
	}
	return p, nil
}

func newProductRow(p domain.CatalogProduct) ProductRow {
	row := ProductRow{
		Name:      p.Name,
		Pack:      fmt.Sprintf("%s %s", formatAmount(p.PackQuantity), p.Unit),
		Nutrition: "No nutrition label",
		Aisle:     p.Aisle,
		Price:     fmt.Sprintf("€%.2f", p.Price),
		Available: p.Available,
		Seen:      p.AvailabilitySeen.Local().Format("2 Jan 15:04"),
	}
	if n := p.Nutrition; n != nil {
		per := "per 100 " + string(p.Unit)
		if p.Unit == domain.Pieces {
			per = "per piece"
		}
		row.Nutrition = fmt.Sprintf("%s kcal · %s g protein %s", formatAmount(n.Kcal), formatAmount(n.ProteinG), per)
	}
	return row
}

func formatAmount(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Seed starts a Run seeding the Known Catalog, and shows its Trace.
func (h Handlers) Seed(w http.ResponseWriter, r *http.Request) {
	id, err := h.Service.StartSeed(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "start catalog seed", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/runs/%d", id), http.StatusSeeOther)
}

// Propose starts a Run proposing Product Mappings, and shows its Trace.
func (h Handlers) Propose(w http.ResponseWriter, r *http.Request) {
	id, err := h.Service.StartProposal(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "start product mapping proposal", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/runs/%d", id), http.StatusSeeOther)
}

// SetMapping sets, confirms or removes an Ingredient's Product Mapping.
// Under htmx it answers with the Ingredient's row alone.
func (h Handlers) SetMapping(w http.ResponseWriter, r *http.Request) {
	ingredientID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	reject := func(rejected MappingRow) {
		if httpx.IsHTMX(r) {
			h.renderMapping(w, r, ingredientID, &rejected, http.StatusUnprocessableEntity)
			return
		}
		h.render(w, r, &rejected, http.StatusUnprocessableEntity)
	}
	var productID int64
	if v := r.PostForm.Get("catalog_product"); v != "" {
		var err error
		if productID, err = strconv.ParseInt(v, 10, 64); err != nil {
			reject(MappingRow{IngredientID: ingredientID, Error: "Choose a Catalog Product"})
			return
		}
	}
	var invalid *MappingError
	switch err := h.Service.SetMapping(r.Context(), ingredientID, productID); {
	case errors.As(err, &invalid):
		reject(MappingRow{IngredientID: ingredientID, Selected: productID, Error: invalid.Problem})
		return
	case errors.Is(err, ErrNotFound) && httpx.IsHTMX(r):
		// Its Ingredient or product is gone: take the stale row away.
		w.WriteHeader(http.StatusNotFound)
		return
	case errors.Is(err, ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		httpx.ServerError(w, h.Log, "set product mapping", err)
		return
	}
	if httpx.IsHTMX(r) {
		h.renderMapping(w, r, ingredientID, nil, http.StatusOK)
		return
	}
	http.Redirect(w, r, "/catalog", http.StatusSeeOther)
}
