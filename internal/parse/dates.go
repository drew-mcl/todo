package parse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Due is the result of parsing a date token. Recognised reports whether the text
// was a date token at all; a Recognised result with Explicit set to false means
// the user deliberately asked for no date ("someday", "none").
type Due struct {
	Recognised bool
	Explicit   bool // true when Date is meaningful
	Date       time.Time
}

func noDate() Due { return Due{Recognised: true} }
func dated(t time.Time) Due {
	return Due{Recognised: true, Explicit: true, Date: t}
}

// day truncates t to local midnight so every stored date is a civil date.
func day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday,
}

var months = map[string]time.Month{
	"jan": time.January, "january": time.January,
	"feb": time.February, "february": time.February,
	"mar": time.March, "march": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May,
	"jun": time.June, "june": time.June,
	"jul": time.July, "july": time.July,
	"aug": time.August, "august": time.August,
	"sep": time.September, "sept": time.September, "september": time.September,
	"oct": time.October, "october": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December,
}

// nextWeekday returns the next occurrence of wd. When includeToday is true and
// now already falls on wd, now is returned.
func nextWeekday(now time.Time, wd time.Weekday, includeToday bool) time.Time {
	delta := (int(wd) - int(now.Weekday()) + 7) % 7
	if delta == 0 && !includeToday {
		delta = 7
	}
	return day(now.AddDate(0, 0, delta))
}

func endOfMonth(now time.Time) time.Time {
	y, m, _ := now.Date()
	return day(time.Date(y, m+1, 1, 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1))
}

// ParseDue interprets s as a whole date token. It only succeeds when the entire
// string is a date, never when one merely appears inside it -- that is what lets
// the line parser tell "| today" from "| ship the thing".
func ParseDue(s string, now time.Time) Due {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	if s == "" {
		return Due{}
	}
	now = day(now)

	switch s {
	case "today", "tod", "eod", "now", "asap":
		return dated(now)
	case "tomorrow", "tom", "tmr", "tmw", "tmrw":
		return dated(now.AddDate(0, 0, 1))
	case "yesterday", "yest":
		return dated(now.AddDate(0, 0, -1))
	case "eow", "end of week", "endofweek", "end week", "this week":
		return dated(nextWeekday(now, time.Friday, true))
	case "eom", "end of month", "endofmonth", "end month", "this month":
		return dated(endOfMonth(now))
	case "eoy", "end of year", "endofyear":
		return dated(time.Date(now.Year(), time.December, 31, 0, 0, 0, 0, now.Location()))
	case "next week", "nextweek", "nw":
		return dated(nextWeekday(now, time.Monday, false))
	case "next month", "nextmonth":
		return dated(day(time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())))
	case "someday", "none", "later", "backlog", "-":
		return noDate()
	}

	// Bare and "next"-prefixed weekday names.
	if wd, ok := weekdays[s]; ok {
		return dated(nextWeekday(now, wd, true))
	}
	// "next fri" means the Friday of next week, never this week's.
	if rest, ok := strings.CutPrefix(s, "next "); ok {
		if wd, ok := weekdays[rest]; ok {
			return dated(nextWeekday(nextWeekday(now, time.Monday, false), wd, true))
		}
	}

	// Relative offsets: +3d +2w +1m +1y
	if d, ok := parseOffset(s, now); ok {
		return dated(d)
	}
	// Numeric forms: 2026-12-25, 25/12, 25/12/2026, 25-12
	if d, ok := parseNumericDate(s, now); ok {
		return dated(d)
	}
	// Month-name forms: "25 dec", "dec 25", "25 december"
	if d, ok := parseMonthName(s, now); ok {
		return dated(d)
	}
	return Due{}
}

func parseOffset(s string, now time.Time) (time.Time, bool) {
	if !strings.HasPrefix(s, "+") || len(s) < 3 {
		return time.Time{}, false
	}
	unit := s[len(s)-1]
	n, err := strconv.Atoi(s[1 : len(s)-1])
	if err != nil || n < 0 {
		return time.Time{}, false
	}
	switch unit {
	case 'd':
		return now.AddDate(0, 0, n), true
	case 'w':
		return now.AddDate(0, 0, 7*n), true
	case 'm':
		return now.AddDate(0, n, 0), true
	case 'y':
		return now.AddDate(n, 0, 0), true
	}
	return time.Time{}, false
}

// parseNumericDate handles ISO (yyyy-mm-dd) and day-first slash/dot forms. Day-first
// is deliberate: this is not a US keyboard.
func parseNumericDate(s string, now time.Time) (time.Time, bool) {
	if t, err := time.ParseInLocation("2006-01-02", s, now.Location()); err == nil {
		return t, true
	}
	sep := ""
	for _, c := range []string{"/", ".", "-"} {
		if strings.Contains(s, c) {
			sep = c
			break
		}
	}
	if sep == "" {
		return time.Time{}, false
	}
	parts := strings.Split(s, sep)
	if len(parts) < 2 || len(parts) > 3 {
		return time.Time{}, false
	}
	d, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || d < 1 || d > 31 || m < 1 || m > 12 {
		return time.Time{}, false
	}
	year := now.Year()
	if len(parts) == 3 {
		y, err := strconv.Atoi(parts[2])
		if err != nil {
			return time.Time{}, false
		}
		if y < 100 {
			y += 2000
		}
		year = y
	}
	t := time.Date(year, time.Month(m), d, 0, 0, 0, 0, now.Location())
	// Guard against 31/02 rolling into March.
	if t.Day() != d || t.Month() != time.Month(m) {
		return time.Time{}, false
	}
	if len(parts) == 2 && t.Before(now) {
		t = t.AddDate(1, 0, 0)
	}
	return t, true
}

func parseMonthName(s string, now time.Time) (time.Time, bool) {
	f := strings.Fields(strings.ReplaceAll(s, ",", " "))
	if len(f) != 2 {
		return time.Time{}, false
	}
	var mo time.Month
	var dayStr string
	if m, ok := months[f[0]]; ok {
		mo, dayStr = m, f[1]
	} else if m, ok := months[f[1]]; ok {
		mo, dayStr = m, f[0]
	} else {
		return time.Time{}, false
	}
	dayStr = strings.TrimRight(dayStr, "stndrh") // 1st 2nd 3rd 4th
	d, err := strconv.Atoi(dayStr)
	if err != nil || d < 1 || d > 31 {
		return time.Time{}, false
	}
	t := time.Date(now.Year(), mo, d, 0, 0, 0, 0, now.Location())
	if t.Day() != d {
		return time.Time{}, false
	}
	if t.Before(now) {
		t = t.AddDate(1, 0, 0)
	}
	return t, true
}

// FormatDue renders a date the way the list and preview show it: relative for the
// near term, absolute beyond that.
func FormatDue(d, now time.Time) string {
	d, now = day(d), day(now)
	switch days := int(d.Sub(now).Hours() / 24); {
	case days == 0:
		return "Today"
	case days == 1:
		return "Tomorrow"
	case days == -1:
		return "Yesterday"
	case days < -1:
		return fmt.Sprintf("%d days overdue", -days)
	case days < 7:
		return d.Format("Mon")
	case d.Year() == now.Year():
		return d.Format("Mon 2 Jan")
	default:
		return d.Format("2 Jan 2006")
	}
}
