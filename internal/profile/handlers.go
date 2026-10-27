package profile

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve the Profile page.
type Handlers struct {
	Service *Service
	Log     *slog.Logger
}

func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	p, found, err := h.Service.Get(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load profile", err)
		return
	}
	f := ProfileForm{Values: map[string]string{}, Saved: r.URL.Query().Has("saved")}
	if found {
		f.Values = profileValues(p)
	} else {
		// A new Profile offers the default Slot times.
		for _, k := range domain.SlotKinds {
			f.Values[slotTimeField(k)] = p.SlotTimes[k].String()
		}
	}
	templ.Handler(ProfilePage(f)).ServeHTTP(w, r)
}

func (h Handlers) Save(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	typed := map[string]string{}
	for name := range r.PostForm {
		typed[name] = r.PostForm.Get(name)
	}
	p, problems := parseProfile(r.PostForm)
	if len(problems) > 0 {
		h.renderForm(w, r, ProfileForm{Values: typed, Errors: problems}, http.StatusUnprocessableEntity)
		return
	}
	if err := h.Service.Save(r.Context(), p); err != nil {
		// Keep what was typed rather than swapping a bare error over the form.
		h.Log.Error("save profile", "err", err)
		h.renderForm(w, r, ProfileForm{Values: typed, Failed: true}, http.StatusInternalServerError)
		return
	}
	if !httpx.IsHTMX(r) {
		// Redirect, so reloading the page doesn't resubmit the form.
		http.Redirect(w, r, "/profile?saved", http.StatusSeeOther)
		return
	}
	h.renderForm(w, r, ProfileForm{Values: profileValues(p), Saved: true}, http.StatusOK)
}

// renderForm answers a submission: HTMX swaps the form in place,
// a plain browser gets the whole page.
func (h Handlers) renderForm(w http.ResponseWriter, r *http.Request, f ProfileForm, status int) {
	c := ProfilePage(f)
	if httpx.IsHTMX(r) {
		c = ProfileFormFragment(f)
	}
	templ.Handler(c, templ.WithStatus(status)).ServeHTTP(w, r)
}
