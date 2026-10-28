// Package catalog is the Known Catalog: the Catalog Products the planner
// knows about, seeded by a Run from the user's Esselunga order history and
// brought up to date by a refresh Run the user starts, and
// the Product Mappings from Ingredients to them, which the user sets and
// the agent proposes.
package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/esselunga"
	"github.com/simopzz/dandori/internal/runs"
)

// ErrNotFound means there is no such Ingredient or Catalog Product.
var ErrNotFound = errors.New("no such ingredient or catalog product")

// MappingError means a Product Mapping breaks the domain rules.
type MappingError struct{ Problem string }

func (e *MappingError) Error() string { return e.Problem }

// Product is a Catalog Product as stored.
type Product struct {
	ID int64
	domain.CatalogProduct
}

// Mapping is an Ingredient with its Product Mapping: ProductID is 0 when it
// has none. Proposed is true while the agent's proposal awaits the user.
type Mapping struct {
	IngredientID int64
	Ingredient   domain.Ingredient
	ProductID    int64
	ProductName  string
	Proposed     bool
}

// Config is how the Known Catalog reaches Esselunga.
type Config struct {
	// Store is the user's Esselunga session, throttled.
	Store *esselunga.Throttled
	// Orders is how many of the latest orders a seed reads.
	Orders int
	// Language is the Content Language Ingredient names are written in.
	Language domain.ContentLanguage
	// Now is the clock availability is seen by.
	Now func() time.Time
}

// Service is the Known Catalog's use cases.
type Service struct {
	pool *pgxpool.Pool
	runs *runs.Engine
	cfg  Config
}

// New creates the Service; its Runs go through engine.
func New(pool *pgxpool.Pool, engine *runs.Engine, cfg Config) *Service {
	return &Service{pool: pool, runs: engine, cfg: cfg}
}

// Orders is how many of the latest orders a seed reads.
func (s *Service) Orders() int { return s.cfg.Orders }

// Products loads every Catalog Product, by aisle and name.
func (s *Service) Products(ctx context.Context) ([]Product, error) {
	return loadProducts(ctx, s.pool)
}

// Mappings loads every Ingredient, by name, with its Product Mapping.
func (s *Service) Mappings(ctx context.Context) ([]Mapping, error) {
	return loadMappings(ctx, s.pool)
}

// SetMapping makes the Catalog Product with productID the Product Mapping
// of the Ingredient with ingredientID, confirming it if it was proposed;
// productID 0 removes the mapping.
func (s *Service) SetMapping(ctx context.Context, ingredientID, productID int64) error {
	_, ing, found, err := loadIngredient(ctx, s.pool, ingredientID, "")
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	if productID != 0 {
		p, found, err := loadProduct(ctx, s.pool, productID, "")
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if problem := domain.MappingProblem(ing, p.CatalogProduct); problem != "" {
			return &MappingError{Problem: problem}
		}
	}
	return setMapping(ctx, s.pool, ingredientID, productID)
}

// StartSeed starts a Run seeding the Known Catalog from the latest orders,
// then proposing a Product Mapping for each Ingredient without one.
func (s *Service) StartSeed(ctx context.Context) (int64, error) {
	brief, err := s.brief(ctx)
	if err != nil {
		return 0, err
	}
	return s.runs.Start(ctx, seedKind, s.seedJob(brief))
}

// StartRefresh starts a Run bringing every Catalog Product up to date with
// what Esselunga shows for it now: pack size, nutrition and availability.
func (s *Service) StartRefresh(ctx context.Context) (int64, error) {
	brief, err := s.brief(ctx)
	if err != nil {
		return 0, err
	}
	return s.runs.Start(ctx, refreshKind, s.refreshJob(brief))
}

// StartProposal starts a Run proposing a Product Mapping for each
// Ingredient without one, from the Known Catalog as it is.
func (s *Service) StartProposal(ctx context.Context) (int64, error) {
	brief, err := s.brief(ctx)
	if err != nil {
		return 0, err
	}
	return s.runs.Start(ctx, proposalKind, s.proposalJob(brief))
}

// Run loads the Known Catalog's Run with id, a seed, refresh or proposal,
// with its Trace. found is false when there is no such Run of the Known
// Catalog.
func (s *Service) Run(ctx context.Context, id int64) (run runs.Run, found bool, err error) {
	run, found, err = s.runs.Get(ctx, id)
	if err != nil || !found || (run.Kind != seedKind && run.Kind != refreshKind && run.Kind != proposalKind) {
		return runs.Run{}, false, err
	}
	run.Events, err = s.runs.Trace(ctx, id, 0)
	return run, err == nil, err
}

// LastRun loads the Known Catalog's newest Run, a seed, refresh or
// proposal, with its Trace. found is false before the first.
func (s *Service) LastRun(ctx context.Context) (runs.Run, bool, error) {
	return s.runs.Latest(ctx, seedKind, refreshKind, proposalKind)
}
