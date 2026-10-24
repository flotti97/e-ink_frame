package clock

import (
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 10, 24, h, m, 30, 0, time.Local) }

func TestPhraseInFiveMinuteSteps(t *testing.T) {
	want := []string{
		"zwölf",
		"fünf nach zwölf",
		"zehn nach zwölf",
		"viertel über zwölf",
		"zehn vor halb eins",
		"fünf vor halb eins",
		"halb eins",
		"fünf nach halb eins",
		"zwanzig vor eins",
		"dreiviertel eins",
		"zehn vor eins",
		"fünf vor eins",
	}
	for i, w := range want {
		for _, m := range []int{5 * i, 5*i + 4} {
			if got := Phrase(at(12, m)); got != w {
				t.Errorf("12:%02d = %q, want %q", m, got, w)
			}
		}
	}
}

func TestPhraseUsesTwelveHours(t *testing.T) {
	for _, c := range []struct {
		h, m int
		want string
	}{
		{0, 0, "zwölf"},
		{1, 0, "eins"},
		{13, 15, "viertel über eins"},
		{23, 30, "halb zwölf"},
		{23, 55, "fünf vor zwölf"},
		{11, 45, "dreiviertel zwölf"},
	} {
		if got := Phrase(at(c.h, c.m)); got != c.want {
			t.Errorf("%02d:%02d = %q, want %q", c.h, c.m, got, c.want)
		}
	}
}

func TestNextIsTheFollowingFiveMinuteMark(t *testing.T) {
	for _, c := range []struct{ h, m, wantH, wantM int }{
		{12, 0, 12, 5},
		{12, 4, 12, 5},
		{12, 57, 13, 0},
		{23, 55, 0, 0},
	} {
		got := Next(at(c.h, c.m))
		if got.Hour() != c.wantH || got.Minute() != c.wantM || got.Second() != 0 {
			t.Errorf("Next(%02d:%02d) = %s", c.h, c.m, got.Format("15:04:05"))
		}
	}
}
