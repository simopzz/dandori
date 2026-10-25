package domain

import (
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
	if s.Quantity <= 0 {
		problems["Quantity"] = "Must be more than 0"
	}
	if msg := s.Location.Validate(); msg != "" {
		problems["Location"] = msg
	}
	return problems
}
