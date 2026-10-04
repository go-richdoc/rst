package rst

import "testing"

// This case lives in its own FILE, and that is the point rather than tidiness.
// It calls isStandaloneURI, an unexported helper the fix introduces -- so with
// the fix stashed the package's tests do not COMPILE, and "go test" reports a
// build failure with no assertion lines at all. Read as "0 failures", that looks
// like a test which does not discriminate, and it silently disables the control
// for every other case in the same file.
//
// Keeping the behavioural witness (standalone_test.go, public API only) apart
// from the unit test of the new internal keeps "each case fails without the fix"
// checkable for the one that can be.

// TestIsStandaloneURI exercises the rule directly, including the shapes no corpus
// file happens to have.
func TestIsStandaloneURI(t *testing.T) {
	cases := map[string]bool{
		"https://example.com/": true,
		"http://x":             true,
		"mailto:a@b":           true,
		"ftp://h/p":            true,
		"a+b-c.d:x":            true, // the scheme grammar is [a-zA-Z][a-zA-Z0-9.+-]*
		"a@b.com":              true,
		"py-code.org":          false,
		"docs/index.html":      false,
		"":                     false,
		"has space:x":          false,
		"@leading":             false,
		"trailing@":            false,
		"1nvalid:x":            false, // a scheme must start with a letter
	}
	for in, want := range cases {
		if got := isStandaloneURI(in); got != want {
			t.Errorf("isStandaloneURI(%q) = %v, want %v", in, got, want)
		}
	}
}
