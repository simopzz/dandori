package kitchen

import (
	"log/slog"
	"net/http"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers bring the signed-in User's kitchen up to now before every page, so a page is
// never wrong because the scheduler was late, and serve the Kitchen page.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// UpToNow brings the signed-in User's kitchen up to now, then serves next.
func (h Handlers) UpToNow(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h.Service.BringUpToNow(r.Context(), h.Clock.Now()); err != nil {
			httpx.ServerError(w, r, h.Log, "bring the kitchen up to now", err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Show is the Kitchen page.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	httpx.Render(w, r, h.Log, http.StatusOK, page(h.Clock.Now()))
}
