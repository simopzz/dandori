package apptest

import "testing"

func TestSelectWithNothingSelectedSubmitsItsFirstOption(t *testing.T) {
	page := `<form id="f" action="/x"><select name="unit"><option value="">choose</option><option value="g">g</option></select></form>`
	if got := Form(t, page, "f").Values["unit"]; len(got) != 1 || got[0] != "" {
		t.Errorf("unit = %q, want the first option's empty value", got)
	}
}

func TestChooseChecksTheRadioButtonLabelledSo(t *testing.T) {
	page := `<form id="f" action="/x">
		<label><input type="radio" name="lang" value="en" checked/><span>English</span></label>
		<label><input type="radio" name="lang" value="it"/><span>Italiano</span></label>
	</form>`
	if got := Choose(t, page, "f", "Italiano").Values["lang"]; len(got) != 1 || got[0] != "it" {
		t.Errorf("lang = %q, want only it", got)
	}
}
