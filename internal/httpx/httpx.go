// Package httpx holds the small HTTP helpers every feature's handlers share.
package httpx

import (
	"log/slog"
	"net/http"
	"strconv"
)

// PathID reads the {id} path value, answering 404 if it is not a number.
func PathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

// ServerError logs err as what failed and answers 500 without its details.
func ServerError(w http.ResponseWriter, log *slog.Logger, what string, err error) {
	log.Error(what, "err", err)
	http.Error(w, "Something went wrong; see the server log.", http.StatusInternalServerError)
}
