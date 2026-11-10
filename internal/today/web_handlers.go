package today

import (
	"log/slog"
	"net/http"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve Today.
type Handlers struct {
	Service *Service
	Clock   clock.Clock
	Log     *slog.Logger
}

// Show is Today at the clock's now.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	now := h.Clock.Now()
	day, err := h.Service.At(r.Context(), now)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load Today", err)
		return
	}
	httpx.Render(w, r, h.Log, http.StatusOK, page(now, day))
}
