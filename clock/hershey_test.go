package clock

import "testing"

func TestParseJHFReadsExtentsAndStrokes(t *testing.T) {
	// Space, then "!" as two strokes, wrapped over two lines.
	font, err := parseJHF("12345  1JZ\n12345  6MWRFRT RRY\nRZ\n")
	if err != nil {
		t.Fatal(err)
	}
	if g := font[' ']; g.left != -8 || g.right != 8 || len(g.strokes) != 0 {
		t.Errorf("space = %+v", g)
	}
	g := font['!']
	if g.left != -5 || g.right != 5 || len(g.strokes) != 2 ||
		g.strokes[0][1] != (point{0, 2}) || len(g.strokes[1]) != 2 {
		t.Errorf("! = %+v", g)
	}
}

func TestEmbeddedFontHasLowerCase(t *testing.T) {
	for _, c := range "abcdefghijklmnopqrstuvwxyz" {
		if len(script[c].strokes) == 0 {
			t.Errorf("no strokes for %q", c)
		}
	}
}
