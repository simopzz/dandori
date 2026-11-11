package domain_test

import (
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

func TestATimeIsWrittenInTheLanguage(t *testing.T) {
	for _, tt := range []struct {
		at     domain.TimeOfDay
		en, it string
	}{
		{0, "12:00 am", "00:00"},
		{5, "12:05 am", "00:05"},
		{8 * 60, "8:00 am", "08:00"},
		{12 * 60, "12:00 pm", "12:00"},
		{13 * 60, "1:00 pm", "13:00"},
		{23*60 + 59, "11:59 pm", "23:59"},
	} {
		if got := tt.at.In(domain.English); got != tt.en {
			t.Errorf("%d in English = %q, want %q", tt.at, got, tt.en)
		}
		if got := tt.at.In(domain.Italian); got != tt.it {
			t.Errorf("%d in Italian = %q, want %q", tt.at, got, tt.it)
		}
	}
	at := time.Date(2026, 11, 2, 13, 0, 5, 0, time.UTC)
	if got := domain.FormatTime(domain.English, at, true); got != "1:00:05 pm" {
		t.Errorf("English with seconds = %q", got)
	}
	if got := domain.FormatTime(domain.Italian, at, true); got != "13:00:05" {
		t.Errorf("Italian with seconds = %q", got)
	}
}

func TestATimeIsReadInEitherForm(t *testing.T) {
	for typed, want := range map[string]domain.TimeOfDay{
		"13:00": 13 * 60, "1:00 pm": 13 * 60, "1pm": 13 * 60, "1:00 PM": 13 * 60, "1 p.m.": 13 * 60,
		"12am": 0, "12:00 am": 0, "12pm": 12 * 60, "12:30pm": 12*60 + 30,
		"0:00": 0, "00:00": 0, "23:59": 23*60 + 59, "8": 8 * 60, "8am": 8 * 60,
		"1300": 13 * 60, "930": 9*60 + 30, "1.30pm": 13*60 + 30,
	} {
		if got, p := domain.ParseTimeOfDay(typed); p != "" || got != want {
			t.Errorf("ParseTimeOfDay(%q) = %d, %q; want %d", typed, got, p, want)
		}
	}
	for typed, want := range map[string]domain.Problem{
		"": domain.Required, "  ": domain.Required,
		"24:00": domain.NotATime, "12:60": domain.NotATime, "13pm": domain.NotATime, "0am": domain.NotATime,
		"noon": domain.NotATime, "1:0": domain.NotATime,
	} {
		if _, p := domain.ParseTimeOfDay(typed); p != want {
			t.Errorf("ParseTimeOfDay(%q) problem = %q, want %q", typed, p, want)
		}
	}
}

func TestADateIsWrittenAndReadTheWayTheLanguageDoes(t *testing.T) {
	day := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	if got := domain.FormatDate(domain.Italian, day); got != "02/11/2026" {
		t.Errorf("Italian = %q", got)
	}
	if got := domain.FormatDate(domain.English, day); got != "11/02/2026" {
		t.Errorf("English = %q", got)
	}
	for _, tt := range []struct {
		l     domain.Language
		typed string
		want  time.Time
		p     domain.Problem
	}{
		{domain.Italian, "2/11/2026", day, ""},
		{domain.English, "11-2-2026", day, ""},
		{domain.Italian, "31/11/2026", time.Time{}, domain.NotADate},
		{domain.English, "13/02/2026", time.Time{}, domain.NotADate},
		{domain.English, "", time.Time{}, domain.Required},
	} {
		if got, p := domain.ParseDate(tt.l, tt.typed); p != tt.p || !got.Equal(tt.want) {
			t.Errorf("ParseDate(%s, %q) = %v, %q; want %v, %q", tt.l, tt.typed, got, p, tt.want, tt.p)
		}
	}
}
