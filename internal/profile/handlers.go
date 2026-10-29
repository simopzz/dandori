package profile

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve the Profile page: the Profile and the Planning Brief, each
// saved with its own form.
type Handlers struct {
	Service *Service
	Log     *slog.Logger
}

func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	f, err := h.storedProfile(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load profile", err)
		return
	}
	b, err := h.storedBrief(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load planning brief", err)
		return
	}
	f.Saved = r.URL.Query().Has("saved")
	b.Saved = r.URL.Query().Has("brief-saved")
	h.renderPage(w, r, f, b, http.StatusOK)
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
		h.renderProfile(w, r, ProfileForm{Values: typed, Errors: problems}, http.StatusUnprocessableEntity)
		return
	}
	if err := h.Service.Save(r.Context(), p); err != nil {
		// Keep what was typed rather than swapping a bare error over the form.
		h.Log.Error("save profile", "err", err)
		h.renderProfile(w, r, ProfileForm{Values: typed, Failed: true}, http.StatusInternalServerError)
		return
	}
	// Redirect, so reloading the page doesn't resubmit the form.
	http.Redirect(w, r, "/profile?saved", http.StatusSeeOther)
}

func (h Handlers) SaveBrief(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	brief := r.PostForm.Get("brief")
	if err := h.Service.SaveBrief(r.Context(), brief); err != nil {
		// Keep what was typed, beside the Profile as stored.
		h.Log.Error("save planning brief", "err", err)
		f, loadErr := h.storedProfile(r.Context())
		if loadErr != nil {
			httpx.ServerError(w, h.Log, "load profile", loadErr)
			return
		}
		h.renderPage(w, r, f, BriefForm{Brief: brief, Failed: true}, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/profile?brief-saved", http.StatusSeeOther)
}

// renderProfile answers a Profile submission that wasn't saved with the
// whole page, the Planning Brief as stored.
func (h Handlers) renderProfile(w http.ResponseWriter, r *http.Request, f ProfileForm, status int) {
	b, err := h.storedBrief(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "load planning brief", err)
		return
	}
	h.renderPage(w, r, f, b, status)
}

func (h Handlers) renderPage(w http.ResponseWriter, r *http.Request, f ProfileForm, b BriefForm, status int) {
	templ.Handler(ProfilePage(f, b), templ.WithStatus(status)).ServeHTTP(w, r)
}

// storedProfile is the Profile form as stored.
func (h Handlers) storedProfile(ctx context.Context) (ProfileForm, error) {
	p, found, err := h.Service.Get(ctx)
	if err != nil {
		return ProfileForm{}, err
	}
	f := ProfileForm{Values: map[string]string{}}
	if found {
		f.Values = profileValues(p)
	} else {
		// A new Profile offers the default Slot times.
		for _, k := range domain.SlotKinds {
			f.Values[slotTimeField(k)] = p.SlotTimes[k].String()
		}
	}
	return f, nil
}

// storedBrief is the Planning Brief form as stored.
func (h Handlers) storedBrief(ctx context.Context) (BriefForm, error) {
	brief, err := h.Service.Brief(ctx)
	return BriefForm{Brief: brief}, err
}
