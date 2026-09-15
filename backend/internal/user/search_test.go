package user

import "testing"

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"alice":      "alice",
		"50%":        `50\%`,
		"a_b":        `a\_b`,
		`back\slash`: `back\\slash`,
		"%_\\":       `\%\_\\`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Fatalf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
