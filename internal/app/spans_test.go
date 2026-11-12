package app_test

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
	"github.com/simopzz/dandori/internal/telemetry"
)

// startWithSpans starts the app with its spans going to an in-memory exporter, in place
// of Grafana Cloud, described by the resource production's are.
func startWithSpans(t *testing.T) (*apptest.Harness, *tracetest.InMemoryExporter) {
	t.Helper()
	opt, spans := withSpans(t)
	return apptest.Start(t, opt), spans
}

// withSpans sends the app's spans to the in-memory exporter it returns.
func withSpans(t *testing.T) (apptest.Option, *tracetest.InMemoryExporter) {
	t.Helper()
	spans := tracetest.NewInMemoryExporter()
	res, err := telemetry.Resource(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans), sdktrace.WithResource(res))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	return apptest.WithConfig(func(c *app.Config) { c.Spans = tp }), spans
}

// A request is a span named for its route, with a span for each database query it runs
// under it.
func TestARequestIsASpanWithItsQueriesUnderIt(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	spans.Reset()

	h.Get(t, "/profile")

	request := onlySpan(t, spans.GetSpans(), "server", func(s tracetest.SpanStub) bool {
		return s.SpanKind == trace.SpanKindServer
	})
	if request.Name != "GET /profile" {
		t.Errorf("request span is named %q, want %q", request.Name, "GET /profile")
	}
	if !hasQueryUnder(spans.GetSpans(), request) {
		t.Errorf("no query span under the request's span; spans: %s", names(spans.GetSpans()))
	}
}

// The container's health check asks every ten seconds, and every page loads the static
// assets, the service worker and the manifest: their requests are not spans.
func TestTheHealthCheckAndAssetsAreNotSpans(t *testing.T) {
	h, spans := startWithSpans(t)
	spans.Reset()

	for _, path := range []string{"/healthz", "/static/htmx.min.js", "/sw.js", "/manifest.webmanifest"} {
		if res := h.Get(t, path); res.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, res.StatusCode)
		}
		if got := spans.GetSpans(); len(got) > 0 {
			t.Errorf("GET %s made spans: %s", path, names(got))
		}
		spans.Reset()
	}
}

// A signed-in request's span says who the User was, by their id, the Language the page
// spoke, what the browser asked for, whether htmx sent it, boosted, and which build
// served it: never their email. Its response gives its trace id.
func TestARequestSpanSaysWhoAndInWhatLanguage(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.ChooseLanguage(t, h.SignIn(t, "ada@example.com"), "Italiano")
	var userID string
	h.Scan(t, &userID, "SELECT user_id::text FROM profile")
	spans.Reset()

	answered := get(t, h, "/profile", http.Header{
		"Accept-Language": {"it-IT,it;q=0.9,en;q=0.8"}, "Hx-Request": {"true"}, "Hx-Boosted": {"true"},
	})

	request := requestSpan(t, spans)
	want := map[attribute.Key]attribute.Value{
		"dandori.user.id":                     attribute.StringValue(userID),
		"dandori.signed_in":                   attribute.BoolValue(true),
		"dandori.language":                    attribute.StringValue("it"),
		"http.request.header.accept-language": attribute.StringSliceValue([]string{"it-IT,it;q=0.9,en;q=0.8"}),
		"dandori.htmx":                        attribute.BoolValue(true),
		"dandori.htmx.boosted":                attribute.BoolValue(true),
	}
	for key, value := range want {
		if got, ok := attr(request, key); !ok || got != value {
			t.Errorf("%s = %q, want %q", key, got.String(), value.String())
		}
	}
	for _, s := range spans.GetSpans() {
		for _, a := range s.Attributes {
			if strings.Contains(a.Value.String(), "ada@example.com") {
				t.Errorf("%s span's %s holds the User's email", s.Name, a.Key)
			}
		}
	}
	// A test binary isn't stamped with a commit at build, as the image is.
	if v, _ := request.Resource.Set().Value("service.version"); v.AsString() != "dev" {
		t.Errorf("service.version = %q, want dev", v.AsString())
	}
	if got := answered.Get("X-Trace-Id"); got != request.SpanContext.TraceID().String() {
		t.Errorf("X-Trace-Id = %q, want the request span's trace id %s", got, request.SpanContext.TraceID())
	}
}

// A request from no one signed in says so, with no User, in the Language the pages were
// switched to, and not through htmx; one with no Accept-Language records none.
func TestASignedOutRequestSpanHasNoUser(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Get(t, "/sign-in?lang=it")
	spans.Reset()

	h.Get(t, "/sign-in")

	request := requestSpan(t, spans)
	for _, key := range []attribute.Key{"dandori.user.id", "http.request.header.accept-language"} {
		if v, ok := attr(request, key); ok {
			t.Errorf("%s = %q, want none", key, v.String())
		}
	}
	want := map[attribute.Key]attribute.Value{
		"dandori.signed_in":    attribute.BoolValue(false),
		"dandori.language":     attribute.StringValue("it"),
		"dandori.htmx":         attribute.BoolValue(false),
		"dandori.htmx.boosted": attribute.BoolValue(false),
	}
	for key, value := range want {
		if got, ok := attr(request, key); !ok || got != value {
			t.Errorf("%s = %q, want %q", key, got.String(), value.String())
		}
	}
}

// A request the app refuses says why on its span, naming the fields a form refused, and
// is no error: the User is told why, and nothing went wrong.
func TestARefusalSaysWhyAndIsNoError(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignIn(t, "ada@example.com")
	used := h.Auth.LastLink(t, "ada@example.com")
	refused := func(what string, res apptest.Response, want string) {
		t.Helper()
		request := requestSpan(t, spans)
		spans.Reset()
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d, want 422", what, res.StatusCode)
		}
		if got, _ := attr(request, "dandori.refusal"); got.AsString() != want {
			t.Errorf("%s: dandori.refusal = %q, want %q", what, got.AsString(), want)
		}
		if request.Status.Code != codes.Unset {
			t.Errorf("%s: span status %s, want unset", what, request.Status.Code)
		}
	}
	spans.Reset()

	page := h.Get(t, "/language")
	spans.Reset()
	form := apptest.Form(t, page.Body, "language")
	form.Values.Set("language", "fr")
	refused("choosing fr", h.HTMXPostForm(t, form.Action, form.Values), "no Language chosen")

	h.ChooseLanguage(t, h.Get(t, "/language"), "English")
	spans.Reset()
	refused("adding a nameless Ingredient", h.HTMXPostForm(t, "/ingredients", url.Values{
		"name": {""}, "unit": {"g"}, "kcal": {"70"}, "protein": {"6"}, "carbs": {"0.5"}, "fat": {"5"}, "usual": {"fridge"},
	}), "Ingredient refused: name")

	link, ok := strings.CutPrefix(used, h.URL)
	if !ok {
		t.Fatalf("the emailed link %s does not lead to the app at %s", used, h.URL)
	}
	confirm := apptest.Form(t, h.Get(t, link).Body, "confirm")
	spans.Reset()
	refused("a used sign-in link", h.HTMXPostForm(t, confirm.Action, confirm.Values), "sign-in link expired or used")
}

// A Recipe refused names the fields of its Ingredient rows it refused; one whose new
// Ingredient is only asked to be confirmed says so, with no field refused.
func TestARefusedRecipeSaysWhy(t *testing.T) {
	opt, spans := withSpans(t)
	s := apptest.NewScenario(t, opt)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.AddsIngredient(rice)
	noQuantity := riceBowl()
	noQuantity.Ingredients[0].Quantity = "0"
	newTofu := riceBowl()
	newTofu.Ingredients = append(newTofu.Ingredients, apptest.RecipeRow{Ingredient: apptest.Ingredient{Name: "tofu"}, Quantity: "300"})

	for _, c := range []struct {
		name   string
		recipe apptest.Recipe
		want   string
	}{
		{"a quantity of 0", noQuantity, "Recipe refused: ingredient quantity"},
		{"a new Ingredient", newTofu, "Recipe's new Ingredient to confirm"},
	} {
		s.Open("/recipes/new")
		spans.Reset()
		s.PostsRecipeForm(c.recipe)
		s.IsRefused()
		if got, _ := attr(requestSpan(t, spans), "dandori.refusal"); got.AsString() != c.want {
			t.Errorf("%s: dandori.refusal = %q, want %q", c.name, got.AsString(), c.want)
		}
	}
}

// A request that fails on the server is an error, and its page gives the trace id to
// quote in a report.
func TestAServerErrorIsAnErrorAndShowsItsTraceID(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	h.Exec(t, "REVOKE SELECT ON profile FROM authenticated")
	spans.Reset()

	res := h.Get(t, "/profile")

	request := requestSpan(t, spans)
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GET /profile = %d, want 500", res.StatusCode)
	}
	if request.Status.Code != codes.Error {
		t.Errorf("span status %s, want error", request.Status.Code)
	}
	if id := request.SpanContext.TraceID().String(); !strings.Contains(res.Body, id) {
		t.Errorf("the error page %q doesn't show the trace id %s", res.Body, id)
	}
}

// A line the app logs in a signed-in request says whose it was, by their id.
func TestALogLineInARequestSaysWho(t *testing.T) {
	h, _ := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	var userID string
	h.Scan(t, &userID, "SELECT user_id::text FROM profile")
	h.Exec(t, "REVOKE SELECT ON profile FROM authenticated")

	h.Get(t, "/profile")

	var line string
	for l := range strings.Lines(h.Logged()) {
		if strings.Contains(l, "load the User's Language") {
			line = l
		}
	}
	if !strings.Contains(line, "dandori.user.id="+userID) {
		t.Errorf("the error logged = %q, want it with dandori.user.id=%s", line, userID)
	}
}

// A scheduler beat is a span, with the queries that bring each User's kitchen up to now
// under it.
func TestABeatIsASpanWithItsQueriesUnderIt(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	spans.Reset()

	h.Clock.Set(h.Clock.Now().Add(5 * time.Minute))

	beat := onlySpan(t, spans.GetSpans(), "kitchen beat", func(s tracetest.SpanStub) bool {
		return s.Name == "kitchen beat"
	})
	if !hasQueryUnder(spans.GetSpans(), beat) {
		t.Errorf("no query span under the beat's span; spans: %s", names(spans.GetSpans()))
	}
}

// A signed-in request whose session has expired renews it with Supabase Auth: a
// client span under the request's.
func TestRenewingASessionIsASpanUnderTheRequest(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	h.Clock.Set(h.Clock.Now().Add(2 * time.Hour))
	spans.Reset()

	h.Get(t, "/profile")

	request := onlySpan(t, spans.GetSpans(), "server", func(s tracetest.SpanStub) bool {
		return s.SpanKind == trace.SpanKindServer
	})
	renew := onlySpan(t, spans.GetSpans(), "supabase renew session", func(s tracetest.SpanStub) bool {
		return s.Name == "supabase renew session"
	})
	if renew.SpanKind != trace.SpanKindClient || renew.Parent.SpanID() != request.SpanContext.SpanID() {
		t.Errorf("supabase renew session is a %s span under %s, want a client span under the request's", renew.SpanKind, renew.Parent.SpanID())
	}
}

// A fresh server fetches the JWKS from Supabase Auth to check the first access token it
// sees: a client span under that request's.
func TestFetchingTheJWKSIsASpanUnderTheFirstCheckedRequest(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")

	h.SignUp(t, "ada@example.com")

	fetch := onlySpan(t, spans.GetSpans(), "supabase fetch JWKS", func(s tracetest.SpanStub) bool {
		return s.Name == "supabase fetch JWKS"
	})
	parent := onlySpan(t, spans.GetSpans(), "parent", func(s tracetest.SpanStub) bool {
		return s.SpanContext.SpanID() == fetch.Parent.SpanID()
	})
	if fetch.SpanKind != trace.SpanKindClient || parent.SpanKind != trace.SpanKindServer {
		t.Errorf("supabase fetch JWKS is a %s span under %q, want a client span under a request's", fetch.SpanKind, parent.Name)
	}
}

// No span of a call to Supabase Auth holds a secret: the access or refresh
// tokens, the API key, or the User's email.
func TestSupabaseSpansHoldNoSecret(t *testing.T) {
	h, spans := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	secrets := []string{"ada@example.com", "sb_publishable_test"}
	keepTokens := func() {
		for _, c := range h.Client.Jar.Cookies(mustParse(t, h.URL)) {
			if c.Name == "dandori_access" || c.Name == "dandori_refresh" {
				secrets = append(secrets, c.Value)
			}
		}
	}
	keepTokens()
	h.Clock.Set(h.Clock.Now().Add(2 * time.Hour))
	profile := h.Get(t, "/profile") // renews the session
	keepTokens()
	form := apptest.Form(t, profile.Body, "sign-out")
	h.PostForm(t, form.Action, form.Values)

	var called []string
	for _, s := range spans.GetSpans() {
		if !strings.HasPrefix(s.Name, "supabase ") {
			continue
		}
		called = append(called, s.Name)
		recorded := s.Name + " " + s.Status.Description
		for _, a := range s.Attributes {
			recorded += " " + a.Value.String()
		}
		for _, e := range s.Events {
			for _, a := range e.Attributes {
				recorded += " " + a.Value.String()
			}
		}
		for _, secret := range secrets {
			if secret != "" && strings.Contains(recorded, secret) {
				t.Errorf("%s span holds the secret %q: %s", s.Name, secret, recorded)
			}
		}
	}
	want := []string{"supabase send magic link", "supabase verify link", "supabase fetch JWKS", "supabase renew session", "supabase sign out"}
	for _, name := range want {
		if !slices.Contains(called, name) {
			t.Errorf("no %s span to check; supabase spans: %v", name, called)
		}
	}
}

// Supabase Auth is not told the spans' ids: no call to it sends a traceparent
// header, even with a propagator set for the whole process.
func TestSupabaseIsSentNoTraceparent(t *testing.T) {
	was := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(was) })
	h, _ := startWithSpans(t)
	h.Invite("ada@example.com")
	h.SignUp(t, "ada@example.com")
	h.Clock.Set(h.Clock.Now().Add(2 * time.Hour))
	h.Get(t, "/profile")

	if got := h.Auth.Traceparents(); len(got) > 0 {
		t.Errorf("the fake Supabase Auth was sent traceparent headers: %v", got)
	}
}

// onlySpan is the one span that matches, described as what; finding none or several fails
// the test.
func onlySpan(t *testing.T, spans tracetest.SpanStubs, what string, match func(tracetest.SpanStub) bool) tracetest.SpanStub {
	t.Helper()
	var found []tracetest.SpanStub
	for _, s := range spans {
		if match(s) {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d %s spans, want 1; spans: %s", len(found), what, names(spans))
	}
	return found[0]
}

// hasQueryUnder reports whether a child of parent is a query's span: a client's,
// but not a call to Supabase Auth.
func hasQueryUnder(spans tracetest.SpanStubs, parent tracetest.SpanStub) bool {
	for _, s := range spans {
		if s.Parent.SpanID() == parent.SpanContext.SpanID() && s.SpanKind == trace.SpanKindClient && !strings.HasPrefix(s.Name, "supabase ") {
			return true
		}
	}
	return false
}

// names lists the spans' names, for a failure to show.
func names(spans tracetest.SpanStubs) string {
	var n []string
	for _, s := range spans {
		n = append(n, s.Name)
	}
	return strings.Join(n, ", ")
}

// get requests path from the app with header, as a browser or htmx would, returning the
// response's header.
func get(t *testing.T, h *apptest.Harness, path string, header http.Header) http.Header {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = header
	res, err := h.Client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	res.Body.Close()
	return res.Header
}

// requestSpan is the one request's span among those exported.
func requestSpan(t *testing.T, spans *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	return onlySpan(t, spans.GetSpans(), "server", func(s tracetest.SpanStub) bool {
		return s.SpanKind == trace.SpanKindServer
	})
}

// attr is the span's attribute key, if it has one.
func attr(s tracetest.SpanStub, key attribute.Key) (attribute.Value, bool) {
	for _, a := range s.Attributes {
		if a.Key == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

// parseWithSpans is a signed-in User who knows rice, their spans going to the exporter it
// returns, who has pasted pastedLasagne for a parse Run the model answers with steps.
func parseWithSpans(t *testing.T, steps ...fakellm.Step) (*apptest.Scenario, *tracetest.InMemoryExporter) {
	t.Helper()
	opt, spans := withSpans(t)
	s := apptest.NewScenario(t, opt)
	s.Invite("ada@example.com")
	s.SignsIn("ada@example.com")
	s.ChoosesLanguage("English")
	s.Open("/ingredients")
	s.AddsIngredient(rice)
	s.Model.Script(steps...)
	spans.Reset()
	s.StartsParse(pastedLasagne)
	return s, spans
}

// runSpan is the one Run's span among those exported, and the spans of its calls to the
// model under it, in order.
func runSpan(t *testing.T, spans *tracetest.InMemoryExporter) (run tracetest.SpanStub, calls tracetest.SpanStubs) {
	t.Helper()
	all := spans.GetSpans()
	run = onlySpan(t, all, "run parse", func(s tracetest.SpanStub) bool { return s.Name == "run parse" })
	for _, s := range all {
		if strings.HasPrefix(s.Name, "chat ") {
			if s.Parent.SpanID() != run.SpanContext.SpanID() {
				t.Errorf("%s span is not under the Run's", s.Name)
			}
			calls = append(calls, s)
		}
	}
	return run, calls
}

// Each call a Run makes to the model is a client span under the Run's span, itself under
// the request that started it: the model, the tokens and why the model stopped, never
// what it was asked or said. A call that failed is an error span, and so is retried.
func TestEachCallToTheModelIsASpanUnderItsRun(t *testing.T) {
	s, spans := parseWithSpans(t,
		fakellm.Overloaded(),
		fakellm.CallTool("submit_recipe", submittedLasagne()),
		fakellm.Respond(agent.Response{
			StopReason: agent.StopEndTurn,
			Content:    []agent.Block{{Type: agent.BlockText, Text: "Parsed the lasagne."}},
			Usage:      agent.Usage{Input: 1200, Output: 300, Thinking: 250, CacheRead: 4000, CacheWrite: 900},
		}))
	runID, err := strconv.ParseInt(strings.TrimPrefix(s.Page().Path, "/runs/"), 10, 64)
	if err != nil {
		t.Fatalf("the paste landed on %s, not a Run's page", s.Page().Path)
	}
	var userID string
	s.Scan(t, &userID, "SELECT user_id::text FROM profile")
	s.Settle()

	run, calls := runSpan(t, spans)
	request := onlySpan(t, spans.GetSpans(), "POST /recipes/parse", func(st tracetest.SpanStub) bool {
		return st.SpanKind == trace.SpanKindServer && st.SpanContext.SpanID() == run.Parent.SpanID()
	})
	if !strings.HasPrefix(request.Name, "POST ") {
		t.Errorf("the Run's span is under %q, want the request that started it", request.Name)
	}
	for key, value := range map[attribute.Key]attribute.Value{
		"dandori.run.id":     attribute.Int64Value(runID),
		"dandori.run.kind":   attribute.StringValue("parse"),
		"dandori.run.status": attribute.StringValue("succeeded"),
		"dandori.user.id":    attribute.StringValue(userID),
	} {
		if got, ok := attr(run, key); !ok || got != value {
			t.Errorf("run span's %s = %q, want %q", key, got.String(), value.String())
		}
	}
	if run.Status.Code != codes.Unset {
		t.Errorf("a succeeded Run's span status is %s, want unset", run.Status.Code)
	}

	if len(calls) != 3 {
		t.Fatalf("%d spans of calls to the model, want 3: %s", len(calls), names(spans.GetSpans()))
	}
	for _, c := range calls {
		if c.Name != "chat claude-opus-5-5" || c.SpanKind != trace.SpanKindClient {
			t.Errorf("a call to the model is a %s span named %q, want a client span named chat claude-opus-5-5", c.SpanKind, c.Name)
		}
		for key, value := range map[attribute.Key]attribute.Value{
			"gen_ai.operation.name": attribute.StringValue("chat"),
			"gen_ai.request.model":  attribute.StringValue("claude-opus-5-5"),
			"dandori.run.id":        attribute.Int64Value(runID),
		} {
			if got, ok := attr(c, key); !ok || got != value {
				t.Errorf("call span's %s = %q, want %q", key, got.String(), value.String())
			}
		}
	}
	if failed := calls[0]; failed.Status.Code != codes.Error || !strings.Contains(failed.Status.Description, "overloaded") {
		t.Errorf("the overloaded call's span status = %s %q, want an error saying overloaded", failed.Status.Code, failed.Status.Description)
	}
	if got, _ := attr(calls[1], "gen_ai.response.finish_reasons"); got.String() != `["tool_use"]` {
		t.Errorf("the tool call's finish reasons = %s, want [tool_use]", got.String())
	}
	last := calls[2]
	for key, want := range map[attribute.Key]string{
		"gen_ai.response.finish_reasons":           `["end_turn"]`,
		"gen_ai.usage.input_tokens":                "6100", // the uncached 1200, with the cache's
		"gen_ai.usage.cache_read.input_tokens":     "4000",
		"gen_ai.usage.cache_creation.input_tokens": "900",
		"gen_ai.usage.output_tokens":               "300",
		"gen_ai.usage.reasoning.output_tokens":     "250",
	} {
		if got, _ := attr(last, key); got.String() != want {
			t.Errorf("the last call's %s = %q, want %s", key, got.String(), want)
		}
	}
	if last.Status.Code != codes.Unset {
		t.Errorf("the last call's span status is %s, want unset", last.Status.Code)
	}

	for _, sp := range append(calls, run) {
		for _, a := range sp.Attributes {
			for _, said := range []string{"Lasagne for 4", "Parsed the lasagne.", "Brown the mince."} {
				if strings.Contains(a.Value.String(), said) {
					t.Errorf("%s span's %s holds what the model was asked or said: %q", sp.Name, a.Key, said)
				}
			}
		}
	}
}

// A failed Run's span is an error, saying why; a cancelled one's is not, nor is the span
// of the call it cut off.
func TestAFailedRunIsAnErrorSpanAndACancelledOneIsNot(t *testing.T) {
	s, spans := parseWithSpans(t, fakellm.Respond(agent.Response{StopReason: agent.StopRefusal, Refusal: "bio"}))
	s.Settle()
	run, _ := runSpan(t, spans)
	if run.Status.Code != codes.Error || run.Status.Description != "model_refused" {
		t.Errorf("a failed Run's span status = %s %q, want an error saying model_refused", run.Status.Code, run.Status.Description)
	}

	asked := make(chan struct{})
	s.Model.Script(asksAgain(asked, fakellm.Hang()))
	spans.Reset()
	s.StartsParse(pastedLasagne)
	<-asked
	s.Open(s.Page().Path)
	s.CancelsRun(apptest.Form(t, s.Page().Body, "cancel-run"))
	s.Settle()
	run, calls := runSpan(t, spans)
	if got, _ := attr(run, "dandori.run.status"); run.Status.Code != codes.Unset || got.AsString() != "cancelled" {
		t.Errorf("a cancelled Run's span status = %s, dandori.run.status = %q, want unset and cancelled", run.Status.Code, got.AsString())
	}
	if len(calls) != 1 || calls[0].Status.Code != codes.Unset {
		t.Errorf("the cut-off call's span = %v, want one, with no error", calls)
	}
}
