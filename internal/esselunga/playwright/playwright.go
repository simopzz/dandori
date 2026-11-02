// Package playwright is the store port over the Playwright MCP server: it
// drives the user's own logged-in browser, so Dandori never holds their
// Esselunga credentials.
//
// The cart is changed and read through the site's pages, as the user would
// (ADR 0012): a product's page holds the quantity in the cart, which can be
// typed in, and the cart page lists every line. Pages are found by their
// addresses and their controls by their accessible labels. Looking up
// products and reading the order history still call the JSON API the site
// itself calls, from inside its page with the user's session (#106). Every
// such request carries the session's XSRF token, since one without it ends
// the session, and the page path header the site's own requests carry,
// without which some answer 401. An expired session answers 401, and loading a page of the site
// renews it through Esselunga's single sign-on; a 401 after that means the
// user is logged out. cmd/esselunga-smoke checks this against the real site.
package playwright

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/simopzz/dandori/internal/esselunga"
	"github.com/simopzz/dandori/internal/mcp"
)

const (
	site = "https://spesaonline.esselunga.it"
	// home is the page loaded to reach the site and renew the session: it
	// needs the user logged in, so it triggers the single sign-on.
	home = site + "/commerce/nav/supermercato/ordini/precedenti"
	// signOn is how long the site is given to log the user back in through
	// the single sign-on, which bounces through account.esselunga.it, and
	// signOnPoll how often the page is checked meanwhile.
	signOn     = 20 * time.Second
	signOnPoll = 500 * time.Millisecond
)

// Store is the user's Esselunga session in the browser the Playwright MCP
// server drives. It starts the server on first use.
type Store struct {
	command []string

	mu     sync.Mutex
	client *mcp.Client
	onSite bool
}

// New returns a Store that runs command, such as
// "npx @playwright/mcp@0.0.83 --user-data-dir ~/.dandori/browser", as the
// Playwright MCP server. Each operation is one request, or two the site
// itself makes together, so the throttle spaces them all.
func New(command string) *Store {
	return &Store{command: strings.Fields(command)}
}

// Close stops the Playwright MCP server, if it was started.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		return nil
	}
	err := s.client.Close()
	s.client, s.onSite = nil, false
	return err
}

func (s *Store) CheckSession(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ignored json.RawMessage
	return s.api(ctx, "GET", "/commerce/resources/auth/profile/customer", nil, "(d) => true", &ignored)
}

func (s *Store) OrderHistory(ctx context.Context, orders int) ([]esselunga.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []struct {
		ID     string    `json:"id"`
		Slot   time.Time `json:"slot"`
		Status string    `json:"status"`
	}
	err := s.api(ctx, "POST", "/commerce/resources/auth/order/previous", map[string]int{"start": 0, "length": orders},
		`(d) => (d.entities || []).map(e => ({id: String(e.orderId), slot: e.slotStartTime, status: e.displayStatus || ''}))`, &list)
	if err != nil {
		return nil, err
	}
	var history []esselunga.Order
	for _, o := range list {
		if strings.Contains(o.Status, "ANNULLAT") {
			continue // cancelled: nothing was bought
		}
		history = append(history, esselunga.Order{ID: o.ID, DeliveredOn: o.Slot})
	}
	return history, nil
}

func (s *Store) OrderLines(ctx context.Context, orderID string) ([]esselunga.OrderLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var lines []esselunga.OrderLine
	err := s.api(ctx, "GET", "/commerce/resources/auth/order/"+orderID, nil,
		`(d) => (d.orderItems || []).map(i => ({
			Code: i.productCode, Name: (i.productName || '').replace(/\s+/g, ' ').trim(), Quantity: i.quantity, Price: i.price,
			Weight: i.productIntervalValue ? i.productIntervalValue + ' ' + (i.productIntervalUnitScale || '') : '',
		}))`,
		&lines)
	if errors.Is(err, esselunga.ErrNoSuchProduct) {
		err = esselunga.ErrNoSuchOrder
	}
	return lines, err
}

// WaitForLogin opens the site in the browser and waits, checking every
// poll, until the user has logged in there or ctx ends. It never reloads
// the page, so the user can log in undisturbed. Log in in this browser, not
// by opening its profile by hand: Playwright reads the profile's cookies
// with its own key, not the desktop keyring's.
func (s *Store) WaitForLogin(ctx context.Context, poll time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	js, err := requestJS("GET", "/commerce/resources/auth/profile/customer", nil, "(d) => true")
	if err != nil {
		return err
	}
	// Already on the site after a failed session check: loading it again
	// would only interrupt the user.
	if err := s.reach(ctx, false); err != nil {
		return err
	}
	for {
		var res struct {
			Status int `json:"status"`
		}
		// While the page is on the login site, there is no token and
		// nothing is sent. Only a browser that is gone ends the wait.
		err := s.evaluate(ctx, js, &res)
		if s.client == nil {
			return err
		}
		if err == nil && res.Status == 200 {
			return nil
		}
		select {
		case <-time.After(poll):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// productJS turns a product's detail into a Product: the pack quantity and
// nutrition label come as HTML, which the page's DOMParser reads.
const productJS = `(d) => {
	const p = d.displayableProduct || {};
	const info = Object.fromEntries((d.informations || []).map(i => [i.label, i.value]));
	const doc = html => new DOMParser().parseFromString(html || '', 'text/html');
	const clean = s => (s || '').replace(/\s+/g, ' ').trim();
	let quantity = '';
	for (const para of doc(info['Caratteristiche']).querySelectorAll('p')) {
		const strong = para.querySelector('strong');
		if (strong && /^quantit/i.test(clean(strong.textContent))) {
			quantity = clean(para.textContent.slice(strong.textContent.length));
		}
	}
	let per = '', rows = [];
	const table = doc(info['Valori nutrizionali']).querySelector('table');
	if (table) {
		per = clean((table.querySelectorAll('thead th')[1] || {}).textContent);
		rows = [...table.querySelectorAll('tbody tr')]
			.map(tr => [...tr.querySelectorAll('td')].map(td => clean(td.textContent)))
			.filter(c => c.length >= 2 && c[1] && c[1] !== '-')
			.map(c => ({label: c[0], value: c[1]}));
	}
	return {
		Code: String(p.code || ''), Name: clean(p.description), Brand: p.brand || '',
		Quantity: quantity, SoldByWeight: !!p.variableWeight,
		Price: p.discountedPrice ?? p.price ?? 0, Available: !p.outOfStock,
		NutritionPer: per, Nutrition: rows,
	};
}`

func (s *Store) LookUpProduct(ctx context.Context, code string) (esselunga.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p esselunga.Product
	path := "/commerce/resources/displayable/detail/code/" + code
	if err := s.api(ctx, "GET", path, nil, productJS, &p); err != nil {
		return esselunga.Product{}, err
	}
	// The site asks for both together on a product page. The aisle is a
	// nicety: a product the site gives none is still a product. A session
	// or browser that is gone is not.
	var noAisle *errAPI
	if err := s.api(ctx, "GET", path+"/breadcrumbs", nil,
		`(d) => (d.breadcrumbs || []).filter(b => b.level > 1).map(b => b.label).join(' › ')`, &p.Aisle); err != nil &&
		!errors.As(err, &noAisle) && !errors.Is(err, esselunga.ErrNoSuchProduct) {
		return esselunga.Product{}, err
	}
	return p, nil
}

// settle is how long the site is given to save a quantity typed in: it
// waits half a second for more typing, then saves it with a request that
// leaving the page too soon cancels. pageLoad is how long a page is given to
// show what is asked of it, and pagePoll how often it is checked meanwhile.
const (
	settle   = 2500 * time.Millisecond
	pageLoad = 20 * time.Second
	pagePoll = 500 * time.Millisecond
)

// productPage is how a product's page stands: State is "loading", "gone"
// when the site no longer sells it, "logged out", or "ready", with the
// Quantity of it in the cart and whether it is Available.
type productPage struct {
	State     string `json:"state"`
	Quantity  int    `json:"quantity"`
	Available bool   `json:"available"`
}

// productPageJS reads a product's page. The product's own controls are the
// nearest to its title: the add button, or once it is in the cart the
// quantity it holds, which the products suggested further down have too.
const productPageJS = `() => {
	if (location.pathname.endsWith('/prodotto-non-trovato')) return {state: 'gone'};
	const title = [...document.querySelectorAll('h1')].find(h => h.innerText.trim() !== 'Esselunga Spesa Online');
	if (!title) return {state: 'loading'};
	let box = title;
	while (box.parentElement && !box.querySelector('button.el-add-to-cart-btn-c, input[role="spinbutton"]')) box = box.parentElement;
	const quantity = box.querySelector('input[role="spinbutton"]');
	const add = box.querySelector('button.el-add-to-cart-btn-c');
	if (!quantity && !add) return {state: 'loading'};
	if (!document.querySelector('a[aria-label$="vai al profilo"]')) return {state: 'logged out'};
	return {state: 'ready', quantity: quantity ? Number(quantity.value) || 0 : 0, available: !!quantity || !add.disabled};
}`

// addJS presses the product's own add button, which puts one in the cart.
const addJS = `() => {
	const title = [...document.querySelectorAll('h1')].find(h => h.innerText.trim() !== 'Esselunga Spesa Online');
	let box = title;
	while (box.parentElement && !box.querySelector('button.el-add-to-cart-btn-c')) box = box.parentElement;
	box.querySelector('button.el-add-to-cart-btn-c').click();
	return true;
}`

// typeQuantityJS types a quantity into the product's own quantity, as the
// user would: 0 takes it out of the cart.
const typeQuantityJS = `(n) => {
	const title = [...document.querySelectorAll('h1')].find(h => h.innerText.trim() !== 'Esselunga Spesa Online');
	let box = title;
	while (box.parentElement && !box.querySelector('input[role="spinbutton"]')) box = box.parentElement;
	const input = box.querySelector('input[role="spinbutton"]');
	input.focus();
	input.value = String(n);
	input.dispatchEvent(new Event('input', {bubbles: true}));
	input.dispatchEvent(new Event('change', {bubbles: true}));
	input.blur();
	return true;
}`

func productPath(code string) string {
	// The site ignores the name that follows the code in a product's address.
	return "/commerce/nav/supermercato/store/prodotto/" + code + "/p"
}

// SetCartQuantity types the quantity into the product's page, after adding
// one when the cart holds none, and checks on the page loaded again that
// the cart kept it.
func (s *Store) SetCartQuantity(ctx context.Context, code string, quantity int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.productPage(ctx, code)
	if err != nil {
		return err
	}
	if p.Quantity == quantity {
		return nil
	}
	if !p.Available {
		return fmt.Errorf("Esselunga has product %s out of stock", code) //nolint:staticcheck // product name
	}
	if p.Quantity == 0 {
		var ignored bool
		if err := s.evaluate(ctx, addJS, &ignored); err != nil {
			return err
		}
		// Loading until the cart holds one.
		added := fmt.Sprintf("() => { const p = (%s)(); return p.state === 'ready' && p.quantity === 0 ? {state: 'loading'} : p; }", productPageJS)
		if _, err := s.await(ctx, "one of product "+code+" in the cart", added, &p); err != nil {
			return err
		}
	}
	if quantity != 1 {
		js := fmt.Sprintf("() => (%s)(%d)", typeQuantityJS, quantity)
		var ignored bool
		if err := s.evaluate(ctx, js, &ignored); err != nil {
			return err
		}
	}
	if err := pause(ctx, settle); err != nil {
		return err
	}
	if p, err = s.productPage(ctx, code); err != nil {
		return err
	}
	if p.Quantity != quantity {
		return fmt.Errorf("the Esselunga cart holds %d of product %s, not the %d set", p.Quantity, code, quantity)
	}
	return nil
}

// productPage loads the page of the product with code and reads it.
func (s *Store) productPage(ctx context.Context, code string) (productPage, error) {
	var p productPage
	if err := s.open(ctx, productPath(code), "the page of product "+code, productPageJS, &p); err != nil {
		return p, err
	}
	if p.State == "gone" {
		return p, esselunga.ErrNoSuchProduct
	}
	return p, nil
}

// cartPage is the cart page: its lines, once it shows them all.
type cartPage struct {
	State string               `json:"state"`
	Lines []esselunga.CartLine `json:"lines"`
}

// cartPageJS reads the cart page. Each line shows the product's picture,
// whose address and description name it, and a menu with the quantity in
// it; the badge on the cart button counts what the cart holds, so an empty
// cart is told from one still loading.
const cartPageJS = `() => {
	const badge = document.querySelector('button[aria-label="Anteprima Carrello"]');
	if (!badge) return {state: 'loading'};
	if (!document.querySelector('a[aria-label$="vai al profilo"]')) return {state: 'logged out'};
	const menus = [...document.querySelectorAll('select[aria-label="Quantità"]')];
	if (menus.length === 0 && badge.innerText.trim() !== '0') return {state: 'loading'};
	const lines = [];
	for (const menu of menus) {
		let row = menu;
		while (row.parentElement && !row.querySelector('img[alt^="Apri dettaglio"]')) row = row.parentElement;
		const img = row.querySelector('img[alt^="Apri dettaglio"]');
		const code = ((img && img.getAttribute('src') || '').match(/\/(\d+)\.\w+$/) || [])[1];
		const quantity = Number((menu.value.match(/^number:(\d+)$/) || [])[1]);
		if (!code || !quantity) return {state: 'unreadable'};
		lines.push({Code: code, Name: img.alt.replace(/^Apri dettaglio /, '').replace(/\s+/g, ' ').trim(), Quantity: quantity});
	}
	return {state: 'ready', lines};
}`

// Cart reads the cart page.
func (s *Store) Cart(ctx context.Context) ([]esselunga.CartLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c cartPage
	if err := s.open(ctx, "/commerce/nav/supermercato/checkout/trolley", "the cart page", cartPageJS, &c); err != nil {
		return nil, err
	}
	if c.State == "unreadable" {
		return nil, errors.New("a line of the Esselunga cart shows no product code or quantity")
	}
	return c.Lines, nil
}

// open loads the site's page at path, what, and waits until js, reading
// it, answers a state other than "loading", decoding what it read into out.
// A page showing the user logged out is loaded once more after renewing the
// session, then is ErrLoggedOut. Call it with s.mu held.
func (s *Store) open(ctx context.Context, path, what, js string, out any) error {
	for attempt := 1; ; attempt++ {
		if err := s.reach(ctx, attempt > 1); err != nil {
			return err
		}
		if _, err := s.tool(ctx, "browser_navigate", map[string]any{"url": site + path}); err != nil {
			return err
		}
		state, err := s.await(ctx, what, js, out)
		switch {
		case err != nil:
			return err
		case state == "logged out" && attempt == 1:
			continue // the session may only have expired: renew it
		case state == "logged out":
			return esselunga.ErrLoggedOut
		}
		return nil
	}
}

// await checks the page with js every pagePoll until it answers a state
// other than "loading", decoding what it read into out, or pageLoad has
// passed, and returns the state.
func (s *Store) await(ctx context.Context, what, js string, out any) (string, error) {
	deadline := time.Now().Add(pageLoad)
	for {
		var read json.RawMessage
		err := s.evaluate(ctx, js, &read)
		if s.client == nil {
			return "", err // the browser is gone
		}
		if err == nil {
			var page struct {
				State string `json:"state"`
			}
			if err := json.Unmarshal(read, &page); err != nil {
				return "", fmt.Errorf("read %s: %w", what, err)
			}
			if page.State != "loading" {
				return page.State, json.Unmarshal(read, out)
			}
		}
		// While the page loads it may fail to answer.
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%s did not load on Esselunga", what) //nolint:staticcheck // product name
		}
		if err := pause(ctx, pagePoll); err != nil {
			return "", err
		}
	}
}

// pause waits d, or until ctx ends.
func pause(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// errAPI is a request the site answered with something other than 200.
type errAPI struct {
	path   string
	status int
}

func (e *errAPI) Error() string {
	return fmt.Sprintf("Esselunga answered %s with %d", e.path, e.status)
}

// api calls the site's JSON API at path from inside its page, and decodes
// into out what the JavaScript function transform makes of the answer.
// Call it with s.mu held.
func (s *Store) api(ctx context.Context, method, path string, body any, transform string, out any) error {
	js, err := requestJS(method, path, body, transform)
	if err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		if err := s.reach(ctx, attempt > 1); err != nil {
			return err
		}
		var res struct {
			Status int             `json:"status"`
			Body   json.RawMessage `json:"body"`
		}
		if err := s.evaluate(ctx, js, &res); err != nil {
			return err
		}
		switch {
		case res.Status == 200:
			return json.Unmarshal(res.Body, out)
		case res.Status == 401 && attempt == 1:
			continue // the session may only have expired: renew it
		case res.Status == 401:
			return esselunga.ErrLoggedOut
		case res.Status == 404:
			return esselunga.ErrNoSuchProduct
		default:
			return &errAPI{path: path, status: res.Status}
		}
	}
}

// requestJS is the JavaScript function that makes one API request from the
// site's page, with the session's XSRF token, and answers its status and
// what transform makes of its body. Without a token it sends nothing: a
// request without one ends the session.
func requestJS(method, path string, body any, transform string) (string, error) {
	bodyJSON := "null"
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		bodyJSON = string(b)
	}
	return fmt.Sprintf(`async () => {
	const token = decodeURIComponent((document.cookie.match(/(?:^|; )XSRF-ECOM-TOKEN=([^;]*)/) || [])[1] || '');
	if (!token) return {status: 401, body: null};
	const init = {method: %q, headers: {accept: 'application/json, text/plain, */*', 'x-xsrf-token': token, 'x-page-path': 'supermercato'}};
	const body = %s;
	if (body) { init.body = JSON.stringify(body); init.headers['content-type'] = 'application/json;charset=UTF-8'; }
	const r = await fetch(%q, init);
	if (!r.ok) return {status: r.status, body: null};
	return {status: r.status, body: (%s)(await r.json())};
}`, method, bodyJSON, path, transform), nil
}

// reach makes sure the browser is on the site, loading its page again when
// renew is set, which renews an expired session.
func (s *Store) reach(ctx context.Context, renew bool) error {
	if s.client == nil {
		c, err := mcp.Start(ctx, s.command)
		if err != nil {
			return err
		}
		s.client = c
	}
	if s.onSite && !renew {
		return nil
	}
	if _, err := s.tool(ctx, "browser_navigate", map[string]any{"url": home}); err != nil {
		return err
	}
	if err := s.awaitSignOn(ctx); err != nil {
		return err
	}
	s.onSite = true
	return nil
}

// signedOnJS reports whether the page is back on the site with a session
// token, which is when the single sign-on has logged the user in. It asks
// Esselunga nothing.
const signedOnJS = `() => location.hostname === 'spesaonline.esselunga.it' && /(?:^|; )XSRF-ECOM-TOKEN=/.test(document.cookie)`

// awaitSignOn waits until the single sign-on has logged the user back in,
// or signOn has passed: then the user is logged out, which the request that
// follows finds.
func (s *Store) awaitSignOn(ctx context.Context) error {
	deadline := time.Now().Add(signOn)
	for {
		var in bool
		// While the sign-on navigates, the page may be between documents and
		// fail to answer; only a browser that is gone ends the wait.
		err := s.evaluate(ctx, signedOnJS, &in)
		if s.client == nil {
			return err
		}
		if (err == nil && in) || time.Now().After(deadline) {
			return nil
		}
		select {
		case <-time.After(signOnPoll):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// evaluate runs the JavaScript function js in the page and decodes what it
// returns into out.
func (s *Store) evaluate(ctx context.Context, js string, out any) error {
	text, err := s.tool(ctx, "browser_evaluate", map[string]any{"function": js})
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(result(text)), out); err != nil {
		return fmt.Errorf("read what the page returned: %w", err)
	}
	return nil
}

// result is the "### Result" section of a Playwright MCP tool's answer,
// which goes on to say what code it ran and what the page looks like.
func result(text string) string {
	if _, after, ok := strings.Cut(text, "### Result\n"); ok {
		text, _, _ = strings.Cut(after, "\n### ")
	}
	return strings.TrimSpace(text)
}

func (s *Store) tool(ctx context.Context, name string, args any) (string, error) {
	res, err := s.client.CallTool(ctx, name, args)
	if err != nil {
		// The server or its browser may be gone: start afresh next time.
		s.client.Close() //nolint:errcheck // replaced anyway
		s.client, s.onSite = nil, false
		return "", err
	}
	if res.IsError {
		return "", fmt.Errorf("Playwright %s failed: %s", name, res.Text) //nolint:staticcheck // product name
	}
	return res.Text, nil
}
