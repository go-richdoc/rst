package rst

import (
	"strings"
	"testing"
)

// TestAFormulaKeepsItsBackslashes pins a doubling that changed what the formula
// MEANS. ":math:" went through rawRole, which escapes a backslash because a role's
// content is escape-processed in general -- but math is one of exactly three roles
// (with "raw" and "code") whose content keeps its backslashes, so ":math:`\emptyset`"
// came back as ":math:`\\emptyset`": TeX for a line break followed by a word.
//
// 16 formulas in 5 of the 14 corpus files that hold one.
func TestAFormulaKeepsItsBackslashes(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"a TeX command", "A :math:`\\emptyset` here.\n", ":math:`\\emptyset`"},
		{"two commands", "A :math:`\\alpha + \\beta` here.\n", ":math:`\\alpha + \\beta`"},
		{
			// A REAL doubled backslash in the source is TeX's line break and has
			// to stay doubled -- the fix is not "stop at one".
			"a TeX line break stays two",
			"A :math:`x\\\\y` here.\n", ":math:`x\\\\y`",
		},
		{
			// The backtick needs no escaping either: the backslash the author
			// wrote in front of it is what keeps the role from ending there, and
			// it survives the restore, so adding another doubled it too.
			"an escaped backtick",
			"M :math:`a\\`b` end.\n", ":math:`a\\`b`",
		},
		{
			// CONTROL, passes either way: no backslash, nothing to get wrong.
			"a formula with no backslash",
			"A :math:`a + b` here.\n", ":math:`a + b`",
		},
	}
	for _, c := range cases {
		out, _ := reconstruct(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, out)
		}
	}
}

// TestAnotherRoleStillDropsItsEscape is the CONTROL that keeps the fix SPECIFIC.
// The reference consumes the escape for every role but raw, code and math: asked
// about ":sub:`a\b`" it answers "<subscript>ab". If that behaviour had been
// changed along with math's, this fails.
func TestAnotherRoleStillDropsItsEscape(t *testing.T) {
	out, _ := reconstruct(t, "A :sub:`a\\b` here.\n")
	if !strings.Contains(out, ":sub:`ab`") {
		t.Errorf("a role that is not math should have lost its escape:\n%s", out)
	}
	if strings.Contains(out, ":sub:`a\\b`") {
		t.Errorf("the escape came back on a role the reference drops it for:\n%s", out)
	}
}

// TestAMathBlockIsUnaffected is the second control: the DIRECTIVE form never went
// through a role and must stay as it was.
func TestAMathBlockIsUnaffected(t *testing.T) {
	out, _ := reconstruct(t, ".. math::\n\n   H \\beta - r\n")
	if !strings.Contains(out, "H \\beta - r") {
		t.Errorf("the math block's formula changed:\n%s", out)
	}
}
