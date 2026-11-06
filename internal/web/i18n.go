package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/httpx"
)

// Message is a string a page shows, in every Language. Pages show no text of their own:
// each string is a Message, shown with T in the Language of the request, and i18n_test.go
// checks every Message has both.
type Message struct {
	En string
	It string
}

// In is m in l, English for any other.
func (m Message) In(l domain.Language) string {
	if l == domain.Italian {
		return m.It
	}
	return m.En
}

// T is m in the Language of ctx.
func T(ctx context.Context, m Message) string { return m.In(LanguageOf(ctx)) }

// Tf is m in the Language of ctx, as a format filled with args.
func Tf(ctx context.Context, m Message, args ...any) string {
	return fmt.Sprintf(T(ctx, m), args...)
}

type languageKey struct{}

// WithLanguage returns ctx carrying the Language its pages speak.
func WithLanguage(ctx context.Context, l domain.Language) context.Context {
	return context.WithValue(ctx, languageKey{}, l)
}

// LanguageOf is the Language ctx's pages speak: the User's, once chosen, before that the
// one the pages were switched to, and English unless they were.
func LanguageOf(ctx context.Context) domain.Language {
	if l, ok := ctx.Value(languageKey{}).(domain.Language); ok {
		return l
	}
	return domain.English
}

// LanguageNames are each Language's name, written in that Language whatever the page
// speaks, so a User finds their own.
var LanguageNames = map[domain.Language]Message{
	domain.English: {En: "English", It: "English"},
	domain.Italian: {En: "Italiano", It: "Italiano"},
}

var (
	weekdayNames = [7]Message{
		{En: "Sunday", It: "domenica"},
		{En: "Monday", It: "lunedì"},
		{En: "Tuesday", It: "martedì"},
		{En: "Wednesday", It: "mercoledì"},
		{En: "Thursday", It: "giovedì"},
		{En: "Friday", It: "venerdì"},
		{En: "Saturday", It: "sabato"},
	}
	monthNames = [12]Message{
		{En: "January", It: "gennaio"},
		{En: "February", It: "febbraio"},
		{En: "March", It: "marzo"},
		{En: "April", It: "aprile"},
		{En: "May", It: "maggio"},
		{En: "June", It: "giugno"},
		{En: "July", It: "luglio"},
		{En: "August", It: "agosto"},
		{En: "September", It: "settembre"},
		{En: "October", It: "ottobre"},
		{En: "November", It: "novembre"},
		{En: "December", It: "dicembre"},
	}
)

// Date is day's weekday and date in the Language of ctx: "Monday 2 November", "lunedì
// 2 novembre".
func Date(ctx context.Context, day time.Time) string {
	return fmt.Sprintf("%s %d %s", T(ctx, weekdayNames[day.Weekday()]), day.Day(), T(ctx, monthNames[day.Month()-1]))
}

// DateInYear is Date with day's year: "Monday 2 November 2026", "lunedì 2 novembre 2026".
func DateInYear(ctx context.Context, day time.Time) string {
	return fmt.Sprintf("%s %d", Date(ctx, day), day.Year())
}

// Number is v as the request's Language writes it, a decimal comma in Italian.
func Number(ctx context.Context, v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if LanguageOf(ctx) == domain.Italian {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// languageCookie keeps the Language the pages were switched to, for the pages before the
// User has chosen theirs.
const languageCookie = "dandori_language"

// languageParam, in a page's query, switches the pages to that Language.
const languageParam = "lang"

// Languages serves next in the Language the pages were switched to. A request with ?lang=
// switches them: it is remembered, and the page opens again without it. A signed-in
// User's own Language, once chosen, replaces it further in.
func Languages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has(languageParam) {
			if l, ok := domain.ParseLanguage(q.Get(languageParam)); ok {
				KeepLanguage(w, r, l)
			}
			q.Del(languageParam)
			u := *r.URL
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}
		if c, err := r.Cookie(languageCookie); err == nil {
			if l, ok := domain.ParseLanguage(c.Value); ok {
				r = r.WithContext(WithLanguage(r.Context(), l))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// KeepLanguage remembers l as the Language the pages speak in this browser, for when no
// User is signed in.
func KeepLanguage(w http.ResponseWriter, r *http.Request, l domain.Language) {
	http.SetCookie(w, &http.Cookie{
		Name:     languageCookie,
		Value:    string(l),
		Path:     "/",
		MaxAge:   400 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   httpx.Secure(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// switchURL is u switching the pages to l.
func switchURL(u *url.URL, l domain.Language) templ.SafeURL {
	q := u.Query()
	q.Set(languageParam, string(l))
	return templ.SafeURL(u.Path + "?" + q.Encode())
}
