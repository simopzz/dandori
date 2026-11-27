package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// saveUpTo records the signed-in User's kitchen as brought up to `now`, unless it already
// is: it never moves back, and a moment reached is not written again.
func saveUpTo(ctx context.Context, q db.Querier, now time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO kitchen (up_to) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET up_to = excluded.up_to
		WHERE kitchen.up_to < excluded.up_to`,
		now)
	return err
}

// LoadUpTo is how far the signed-in User's kitchen has been brought up to now, `now` when
// it never was or has been brought further. It has no WHERE on purpose, as loadBeat.
func LoadUpTo(ctx context.Context, q db.Querier, now time.Time) (time.Time, error) {
	var upTo time.Time
	err := q.QueryRow(ctx, `SELECT up_to FROM kitchen`).Scan(&upTo)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && upTo.After(now)) {
		return now, nil
	}
	return upTo, err
}

// saveBeat records the scheduler's beat at `now` for the signed-in User, whose kitchen
// row saveUpTo has made, unless a beat as late is recorded.
func saveBeat(ctx context.Context, q db.Querier, now time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE kitchen SET beaten_at = $1
		WHERE beaten_at IS NULL OR beaten_at < $1`,
		now)
	return err
}

// loadBeat is the signed-in User's last beat, if any. It has no WHERE on purpose: run in
// db.AsUser, row-level security leaves only their row.
func loadBeat(ctx context.Context, q db.Querier) (time.Time, bool, error) {
	var at *time.Time
	err := q.QueryRow(ctx, `SELECT beaten_at FROM kitchen`).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil || at == nil {
		return time.Time{}, false, err
	}
	return *at, true, nil
}

// lock holds the signed-in User's kitchen until `q`'s transaction ends, so what one Count
// or move read isn't changed by another before it writes. SaveUpTo made their row.
func lock(ctx context.Context, q db.Querier) error {
	_, err := q.Exec(ctx, `SELECT 1 FROM kitchen FOR UPDATE`)
	return err
}

// LoadMovements is the signed-in User's Kitchen Ledger in the order it happened, those of
// one moment in the order they were written. It has no WHERE on purpose: run in
// db.AsUser, row-level security leaves only their rows.
func LoadMovements(ctx context.Context, q db.Querier) ([]domain.Movement, error) {
	return loadMovements(ctx, q, `TRUE`)
}

// LoadIngredientLedger is the Ledger of the signed-in User's Movements of the Ingredient
// `id`, with the time of their latest Movement of any Ingredient, for a reader or a write
// that needs only `id`. Its max has no WHERE on purpose, as LoadMovements.
func LoadIngredientLedger(ctx context.Context, q db.Querier, id int64) (domain.Ledger, error) {
	movements, err := loadMovements(ctx, q, `ingredient_id = $1`, id)
	if err != nil {
		return domain.Ledger{}, err
	}
	var latest *time.Time
	if err := q.QueryRow(ctx, `SELECT max(happened_at) FROM movement`).Scan(&latest); err != nil {
		return domain.Ledger{}, err
	}
	if latest == nil {
		return domain.IngredientLedger(movements, time.Time{}), nil
	}
	return domain.IngredientLedger(movements, *latest), nil
}

// loadMovements is the signed-in User's Movements that `where` keeps, with `args`, in the
// order LoadMovements gives them.
func loadMovements(ctx context.Context, q db.Querier, where string, args ...any) ([]domain.Movement, error) {
	rows, err := q.Query(ctx, `
		SELECT happened_at, coalesce(ingredient_id, 0), storage_location, coalesce(moved_to, ''), change,
			cause_kind, coalesce(believed, 0), coalesce(stated, 0), coalesce(note, ''),
			ARRAY(SELECT portion_id FROM movement_portion WHERE movement_id = movement.id ORDER BY portion_id)
		FROM movement WHERE `+where+` ORDER BY happened_at, id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Movement, error) {
		var (
			m                   domain.Movement
			location, to, cause string
			change, believed    float64
			stated              float64
		)
		err := row.Scan(&m.At, &m.Key.Ingredient, &location, &to, &change, &cause, &believed, &stated, &m.Note, &m.Portions)
		m.Key.Location, m.To = domain.StorageLocation(location), domain.StorageLocation(to)
		m.Change, m.Cause = domain.QuantityOf(change), domain.Cause(cause)
		m.Believed, m.Stated = domain.QuantityOf(believed), domain.QuantityOf(stated)
		return m, err
	})
}

// saveMovement appends `m` to the signed-in User's Kitchen Ledger, with the Portions it
// touches. Only a Count keeps what was believed and stated, and a blank Storage Location
// to or note, no Ingredient or Prep Session, or a zero Slot, is none.
func saveMovement(ctx context.Context, q db.Querier, m domain.Movement) error {
	var believed, stated *float64
	if m.Cause.Sets() {
		b, s := m.Believed.Number(), m.Stated.Number()
		believed, stated = &b, &s
	}
	var slotOn, slotKind *string
	if !m.Slot.On.IsZero() {
		on, kind := m.Slot.On.Format(time.DateOnly), string(m.Slot.Kind)
		slotOn, slotKind = &on, &kind
	}
	var id int64
	err := q.QueryRow(ctx, `
		INSERT INTO movement (happened_at, ingredient_id, storage_location, moved_to, change, cause_kind, believed, stated, note,
			cause_ref, slot_on, slot_kind)
		VALUES ($1, NULLIF($2::bigint, 0), $3, NULLIF($4, ''), $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10::bigint, 0), $11, $12)
		RETURNING id`,
		m.At, m.Key.Ingredient, string(m.Key.Location), string(m.To), m.Change.Number(), string(m.Cause),
		believed, stated, m.Note, m.PrepSession, slotOn, slotKind).Scan(&id)
	if err != nil || len(m.Portions) == 0 {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO movement_portion (movement_id, portion_id) SELECT $1, unnest($2::bigint[])`, id, m.Portions)
	return err
}

// savePortions saves how many Portions `p` says for the signed-in User, and is their ids.
func savePortions(ctx context.Context, q db.Querier, p domain.NewPortions) ([]int64, error) {
	n := p.Nutrition
	rows, err := q.Query(ctx, `
		INSERT INTO portion (name, kcal, protein, carbs, fat, fibre, sugar)
		SELECT $1, $2, $3, $4, $5, $6, $7 FROM generate_series(1, $8::int)
		RETURNING id`,
		p.Name, n.Kcal.Value, n.Protein.Value, n.Carbs.Value, n.Fat.Value, known(n.Fibre), known(n.Sugar), p.HowMany)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// savePrepared saves the Portions `made` for the signed-in User, each recording its
// Planned Portion, and is their ids by Planned Portion.
func savePrepared(ctx context.Context, q db.Querier, made []domain.PreparedPortion) (map[int64]int64, error) {
	ids := map[int64]int64{}
	for _, p := range made {
		n := p.Nutrition
		var id int64
		err := q.QueryRow(ctx, `
			INSERT INTO portion (name, kcal, protein, carbs, fat, fibre, sugar, planned_portion_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			p.Name, n.Kcal.Value, n.Protein.Value, n.Carbs.Value, n.Fat.Value, known(n.Fibre), known(n.Sugar), p.Planned).Scan(&id)
		if err != nil {
			return nil, err
		}
		ids[p.Planned] = id
	}
	return ids, nil
}

// LoadMadeFrom is the Portion each of the signed-in User's Planned Portions was prepared
// as, by Planned Portion. It has no WHERE on purpose, as LoadMovements.
func LoadMadeFrom(ctx context.Context, q db.Querier) (map[int64]int64, error) {
	rows, err := q.Query(ctx, `SELECT planned_portion_id, id FROM portion WHERE planned_portion_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	made := map[int64]int64{}
	var planned, id int64
	_, err = pgx.ForEachRow(rows, []any{&planned, &id}, func() error {
		made[planned] = id
		return nil
	})
	return made, err
}

// known is `a`, or nil for an unknown value, which is never 0.
func known(a domain.Amount) *float64 {
	if !a.Known {
		return nil
	}
	return &a.Value
}

// amount is the value `v` read, unknown if nil.
func amount(v *float64) domain.Amount {
	if v == nil {
		return domain.Unknown
	}
	return domain.Known(*v)
}

// LoadPortions is the signed-in User's Portions, oldest first. It has no WHERE on
// purpose: run in db.AsUser, row-level security leaves only their rows.
func LoadPortions(ctx context.Context, q db.Querier) ([]domain.Portion, error) {
	rows, err := q.Query(ctx, `SELECT id, name, kcal, protein, carbs, fat, fibre, sugar FROM portion ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Portion, error) {
		var (
			p                    domain.Portion
			kcal, protein, carbs float64
			fat                  float64
			fibre, sugar         *float64
		)
		err := row.Scan(&p.ID, &p.Name, &kcal, &protein, &carbs, &fat, &fibre, &sugar)
		p.Nutrition = domain.Nutrition{
			Kcal: domain.Known(kcal), Protein: domain.Known(protein), Carbs: domain.Known(carbs), Fat: domain.Known(fat),
			Fibre: amount(fibre), Sugar: amount(sugar),
		}
		return p, err
	})
}

// CountPortions is how many Portions the signed-in User has ever had, for the Profile:
// each one's name is a Food Term.
func CountPortions(ctx context.Context, q db.Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM portion`).Scan(&n)
	return n, err
}

// HasMovements says whether any Movement of the signed-in User's refers to the Ingredient
// `id`, for the Ingredients, whose unit is then fixed. It holds the kitchen first, as a
// Count or move does, so none of them writes a Movement of it until `q`'s
// transaction ends.
func HasMovements(ctx context.Context, q db.Querier, id int64) (bool, error) {
	if err := lock(ctx, q); err != nil {
		return false, err
	}
	var has bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM movement WHERE ingredient_id = $1)`, id).Scan(&has)
	return has, err
}

// WithMovements is the ids of the signed-in User's Ingredients that some Movement refers
// to, in `q`'s transaction, for the Ingredients, whose rows link to their Movements.
func WithMovements(ctx context.Context, q db.Querier) (map[int64]bool, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT ingredient_id FROM movement WHERE ingredient_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	with := make(map[int64]bool, len(ids))
	for _, id := range ids {
		with[id] = true
	}
	return with, err
}

// LoadThawed is the signed-in User's thawed Movements from `since` on, in the order they
// happened, each with the Slot it is for, its day in `loc`.
func LoadThawed(ctx context.Context, q db.Querier, since time.Time, loc *time.Location) ([]domain.Movement, error) {
	return loadAssumed(ctx, q, loc, `cause_kind = $1 AND happened_at >= $2`, string(domain.Thawed), since)
}

// LoadAssumed is the signed-in User's Movements assumed at a Slot after `since` and up to
// `now`, in the order they happened, each with its Slot, its day in `now`'s zone.
func LoadAssumed(ctx context.Context, q db.Querier, since, now time.Time) ([]domain.Movement, error) {
	return loadAssumed(ctx, q, now.Location(), `slot_on IS NOT NULL AND happened_at > $1 AND happened_at <= $2`, since, now)
}

// loadAssumed is the signed-in User's Movements that `where` keeps, with `args`, in the
// order they happened, each with its Prep Session and its Slot, its day in `loc`.
func loadAssumed(ctx context.Context, q db.Querier, loc *time.Location, where string, args ...any) ([]domain.Movement, error) {
	rows, err := q.Query(ctx, `
		SELECT happened_at, coalesce(ingredient_id, 0), storage_location, coalesce(moved_to, ''), change, cause_kind,
			coalesce(cause_ref, 0), slot_on, slot_kind,
			ARRAY(SELECT portion_id FROM movement_portion WHERE movement_id = movement.id ORDER BY portion_id)
		FROM movement WHERE `+where+` ORDER BY happened_at, id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Movement, error) {
		var (
			m                         domain.Movement
			location, to, cause, kind string
			change                    float64
			on                        time.Time
		)
		err := row.Scan(&m.At, &m.Key.Ingredient, &location, &to, &change, &cause, &m.PrepSession, &on, &kind, &m.Portions)
		m.Key.Location, m.To = domain.StorageLocation(location), domain.StorageLocation(to)
		m.Change, m.Cause = domain.QuantityOf(change), domain.Cause(cause)
		m.Slot = domain.Slot{On: time.Date(on.Year(), on.Month(), on.Day(), 0, 0, 0, 0, loc), Kind: domain.SlotKind(kind)}
		return m, err
	})
}
