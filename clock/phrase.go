// Package clock tells the time in Austrian German words, in steps of five
// minutes, and draws it onto a Slide.
package clock

import "time"

var hours = [12]string{"zwölf", "eins", "zwei", "drei", "vier", "fünf", "sechs", "sieben", "acht", "neun", "zehn", "elf"}

// Phrase says t in words, rounded down to five minutes: "zehn vor halb
// drei" at 14:20 to 14:24.
func Phrase(t time.Time) string {
	this, next := hours[t.Hour()%12], hours[(t.Hour()+1)%12]
	switch t.Minute() / 5 {
	case 0:
		return this
	case 1:
		return "fünf nach " + this
	case 2:
		return "zehn nach " + this
	case 3:
		return "viertel über " + this
	case 4:
		return "zehn vor halb " + next
	case 5:
		return "fünf vor halb " + next
	case 6:
		return "halb " + next
	case 7:
		return "fünf nach halb " + next
	case 8:
		return "zwanzig vor " + next
	case 9:
		return "dreiviertel " + next
	case 10:
		return "zehn vor " + next
	default:
		return "fünf vor " + next
	}
}

// Next is the first five-minute mark after t, when Phrase changes.
func Next(t time.Time) time.Time {
	return t.Truncate(time.Minute).Add(time.Duration(5-t.Minute()%5) * time.Minute)
}
