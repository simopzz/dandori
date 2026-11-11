package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	NotATime Problem = "not a time"
	NotADate Problem = "not a date"
)

// FormatTime is t's time of day as a Language writes it: "13:00" in Italian, "1:00 pm"
// in English. seconds adds them: "13:00:05", "1:00:05 pm".
func FormatTime(l Language, t time.Time, seconds bool) string {
	h, m, s := t.Clock()
	sec := ""
	if seconds {
		sec = fmt.Sprintf(":%02d", s)
	}
	if l == Italian {
		return fmt.Sprintf("%02d:%02d%s", h, m, sec)
	}
	suffix := "am"
	if h >= 12 {
		suffix = "pm"
	}
	return fmt.Sprintf("%d:%02d%s %s", (h+11)%12+1, m, sec, suffix)
}

// In is t as l writes a time of day.
func (t TimeOfDay) In(l Language) string {
	return FormatTime(l, time.Date(2000, 1, 1, int(t)/60, int(t)%60, 0, 0, time.UTC), false)
}

var typedTime = regexp.MustCompile(`^(\d{1,2})(?:[:.]?(\d{2}))?(?:([ap])\.?m\.?)?$`)

// ParseTimeOfDay reads a time of day as the User typed it, in either form: "13:00",
// "1:00 pm", "1pm", "1300".
func ParseTimeOfDay(typed string) (TimeOfDay, Problem) {
	typed = strings.ToLower(strings.Join(strings.Fields(typed), ""))
	if typed == "" {
		return 0, Required
	}
	m := typedTime.FindStringSubmatch(typed)
	if m == nil {
		return 0, NotATime
	}
	h, _ := strconv.Atoi(m[1])
	min := 0
	if m[2] != "" {
		min, _ = strconv.Atoi(m[2])
	}
	if m[3] != "" {
		if h < 1 || h > 12 {
			return 0, NotATime
		}
		h %= 12
		if m[3] == "p" {
			h += 12
		}
	}
	if h > 23 || min > 59 {
		return 0, NotATime
	}
	return TimeOfDay(h*60 + min), ""
}

// FormatDate is day as a Language writes it in a field: "02/11/2026" in Italian,
// "11/02/2026" in English.
func FormatDate(l Language, day time.Time) string {
	if l == Italian {
		return day.Format("02/01/2006")
	}
	return day.Format("01/02/2006")
}

var typedDate = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})$`)

// ParseDate reads a date as the User typed it in l's order of day and month, with any of
// / . - between them, as a day at midnight UTC.
func ParseDate(l Language, typed string) (time.Time, Problem) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return time.Time{}, Required
	}
	m := typedDate.FindStringSubmatch(typed)
	if m == nil {
		return time.Time{}, NotADate
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	y, _ := strconv.Atoi(m[3])
	day, month := a, b
	if l != Italian {
		day, month = b, a
	}
	d := time.Date(y, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if d.Day() != day || int(d.Month()) != month { // time.Date rolls 31 Nov over
		return time.Time{}, NotADate
	}
	return d, ""
}
