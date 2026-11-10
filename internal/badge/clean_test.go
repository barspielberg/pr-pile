package badge

import "testing"

func TestCleanMakesOutputSafeToDraw(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"In review\n", "In review"},
		{"first\nsecond\n", "first"},
		{"  padded  \r\n", "padded"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b]8;;https://x\x07link\x1b]8;;\x07", "link"},
		{"a\tb", "a b"},
		{"bell\a", "bell"},
		{"‮evil", "evil"},
		{"zero​width", "zerowidth"},
		{"\xff\xfe", "�"},
		{"", ""},
		{"\n\n", ""},
	} {
		if got := Clean([]byte(tc.in)); got != tc.want {
			t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
