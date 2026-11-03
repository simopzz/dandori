package apptest

import "testing"

func TestSelectWithNothingSelectedSubmitsItsFirstOption(t *testing.T) {
	page := `<form id="f" action="/x"><select name="unit"><option value="">choose</option><option value="g">g</option></select></form>`
	if got := Form(t, page, "f").Values["unit"]; len(got) != 1 || got[0] != "" {
		t.Errorf("unit = %q, want the first option's empty value", got)
	}
}
