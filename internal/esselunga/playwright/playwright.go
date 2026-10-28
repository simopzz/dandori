// Package playwright is the store port over the Playwright MCP server: it
// drives the user's own logged-in browser, so Dandori never holds their
// Esselunga credentials.
//
// Rather than click through pages, it calls the JSON API the Esselunga
// site itself calls, from inside the site's page with the user's session.
// Every request carries the session's XSRF token, since one without it ends
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
	// settle is how long the page is given to load and renew the session.
	settle = 5 * time.Second
)

// Store is the user's Esselunga session in the browser the Playwright MCP
// server drives. It starts the server on first use.
type Store struct {
	command []string
	// pace spaces the several requests one operation makes, as the
	// throttle spaces operations.
	pace time.Duration

	mu     sync.Mutex
	client *mcp.Client
	onSite bool
}

// New returns a Store that runs command, such as
// "npx @playwright/mcp@0.0.83 --user-data-dir ~/.dandori/browser", as the
// Playwright MCP server, and spaces its requests pace apart.
func New(command string, pace time.Duration) *Store {
	return &Store{command: strings.Fields(command), pace: pace}
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
		if err := s.wait(ctx); err != nil {
			return nil, err
		}
		order := esselunga.Order{ID: o.ID, DeliveredOn: o.Slot}
		err := s.api(ctx, "GET", "/commerce/resources/auth/order/"+o.ID, nil,
			`(d) => (d.orderItems || []).map(i => ({
				Code: i.productCode, Name: (i.productName || '').replace(/\s+/g, ' ').trim(), Quantity: i.quantity, Price: i.price,
				Weight: i.productIntervalValue ? i.productIntervalValue + ' ' + (i.productIntervalUnitScale || '') : '',
			}))`,
			&order.Lines)
		if err != nil {
			return nil, err
		}
		history = append(history, order)
	}
	return history, nil
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
	if err := s.wait(ctx); err != nil {
		return esselunga.Product{}, err
	}
	// The aisle is a nicety: a product without one is still a product.
	if err := s.api(ctx, "GET", path+"/breadcrumbs", nil,
		`(d) => (d.breadcrumbs || []).filter(b => b.level > 1).map(b => b.label).join(' › ')`, &p.Aisle); errors.Is(err, esselunga.ErrLoggedOut) {
		return esselunga.Product{}, err
	}
	return p, nil
}

// errNoCart is why the cart operations fail: Cart Fill (#12) will add them
// once it is known how the site's cart API sets a quantity.
var errNoCart = errors.New("the Playwright store can't use the cart yet")

func (s *Store) SetCartQuantity(context.Context, string, int) error { return errNoCart }

func (s *Store) Cart(context.Context) ([]esselunga.CartLine, error) { return nil, errNoCart }

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
	if _, err := s.tool(ctx, "browser_wait_for", map[string]any{"time": settle.Seconds()}); err != nil {
		return err
	}
	s.onSite = true
	return nil
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

// wait spaces two requests of one operation.
func (s *Store) wait(ctx context.Context) error {
	select {
	case <-time.After(s.pace):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
