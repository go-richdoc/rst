package rst

import "testing"

// TestStartsWithFieldMarker checks the transcription against docutils' own pattern,
//
//	field_marker=r':(?![: ])([^:\\]|\\.|:(?!([ `]|$)))*(?<! ):( +|$)'
//
// one case per assertion in it. This lives in its own file because it names an
// unexported function the behavioural test does not: a test that cannot compile
// without the change under test reports "[build failed]" and asserts nothing.
func TestStartsWithFieldMarker(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{":field: value", true},
		{":field:", true},
		{":two words: value", true},
		// The shape that found this: a colon followed by a BACKSLASH is a legal
		// name character, and the next colon closes the name.
		{":pep:\\`PEP 522: Allow BlockingIOError on Linux", true},
		// The opening colon may not be followed by a colon or a space.
		{":: literal block marker", false},
		{": not a field", false},
		// A colon inside the name may not be followed by a space, a backtick or
		// the end of the line.
		{":a:`role` text", false},
		{":sub:`x` at the start", false},
		// The name may not END in a space.
		{":name : value", false},
		// No closing colon at all.
		{":just a colon start", false},
		{"", false},
		{":", false},
		{"not a field at all", false},
	}
	for _, c := range cases {
		if got := startsWithFieldMarker(c.in); got != c.want {
			t.Errorf("startsWithFieldMarker(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
