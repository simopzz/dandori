package domain

import (
	"math"
	"slices"
	"time"
)

// StorageLocation is where a Portion or Stock Item is kept.
type StorageLocation string

const (
	Pantry  StorageLocation = "pantry"
	Fridge  StorageLocation = "fridge"
	Freezer StorageLocation = "freezer"
)

// StorageLocations lists every StorageLocation.
var StorageLocations = []StorageLocation{Pantry, Fridge, Freezer}

// Validate returns what is wrong with l, or "" when it is one of the
// StorageLocations.
func (l StorageLocation) Validate() string {
	if !slices.Contains(StorageLocations, l) {
		return "Must be pantry, fridge or freezer"
	}
	return ""
}

// StockItem is a quantity of one Ingredient, in its Unit, held in a
// Storage Location.
type StockItem struct {
	Ingredient Ingredient
	Quantity   float64
	Location   StorageLocation
	// ExpiresOn is the expiry date, or nil when there is none.
	ExpiresOn *time.Time
}

// Validate returns what is wrong with the Stock Item, keyed by field name
// such as "Quantity". A valid Stock Item returns none.
func (s StockItem) Validate() map[string]string {
	problems := map[string]string{}
	switch {
	case math.IsNaN(s.Quantity) || math.IsInf(s.Quantity, 0):
		problems["Quantity"] = "Must be a number"
	case s.Quantity <= 0:
		problems["Quantity"] = "Must be more than 0"
	}
	if msg := s.Location.Validate(); msg != "" {
		problems["Location"] = msg
	}
	return problems
}

// Expired reports whether the Stock Item is past its expiry date on today.
// An expired Stock Item stays in the kitchen, but no Plan uses it.
func (s StockItem) Expired(today time.Time) bool {
	return s.ExpiresOn != nil && s.ExpiresOn.Before(today)
}

// MissingFrom names the Recipe's Ingredients, in its order, that stock
// doesn't cover on today: those whose unexpired Stock Items, across every
// Storage Location, add up to less than the Recipe's quantity as written.
// A Recipe with none missing is covered by stock.
func (r Recipe) MissingFrom(stock []StockItem, today time.Time) []string {
	held := map[string]float64{}
	for _, s := range stock {
		if !s.Expired(today) {
			held[s.Ingredient.Name] += s.Quantity
		}
	}
	var missing []string
	for _, ri := range r.Ingredients {
		if held[ri.Ingredient.Name] < ri.Quantity {
			missing = append(missing, ri.Ingredient.Name)
		}
	}
	return missing
}
