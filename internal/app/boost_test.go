package app_test

import (
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// Every page rides the boost on <body>, as ADR 0009 decides.
func TestEveryPageIsBoosted(t *testing.T) {
	h := apptest.Start(t)

	for _, path := range []string{"/", "/profile", "/stock", "/recipes", "/runs", "/plan"} {
		page := h.Get(t, path).Body
		for _, want := range []string{
			`hx-boost:inherited="true"`,
			`hx-disable:inherited="findAll button"`,
			`hx-status:422:inherited="push:false"`,
			`<meta name="htmx-config" content="defaultSettleDelay:0">`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s lacks %s", path, want)
			}
		}
	}
}
