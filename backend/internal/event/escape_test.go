package event

import "testing"

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"Tashkent":   "Tashkent",
		"100%":       `100\%`,
		"a_b":        `a\_b`,
		`back\slash`: `back\\slash`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Fatalf("escapeLike(%q)=%q want %q", in, got, want)
		}
	}
}
