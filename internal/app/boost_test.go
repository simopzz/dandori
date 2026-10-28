package app_test

import (
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// Every link and form rides the boost on <body>: a form's buttons are
// disabled while its request is in flight, and a rejection keeps the
// address of the page the form was on. Settling is off, so a rejected add
// form's script finds its sheet to reopen. See ADR 0009.
func TestEveryPageIsBoosted(t *testing.T) {
	h := apptest.Start(t)

	for _, path := range []string{"/", "/profile", "/stock", "/recipes", "/runs", "/plan"} {
		page := h.Get(t, path).Body
		if !strings.Contains(page, `<body hx-boost:inherited="true" hx-disable:inherited="findAll button" hx-status:422:inherited="push:false">`) {
			t.Errorf("%s is not boosted with its buttons disabled during a request and rejections kept in place", path)
		}
		if !strings.Contains(page, `<meta name="htmx-config" content="defaultSettleDelay:0">`) {
			t.Errorf("%s lets htmx settle swaps, which hides a swapped-in page from its own scripts", path)
		}
	}
}
