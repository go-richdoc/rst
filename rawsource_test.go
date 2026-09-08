package rst

import (
	"strings"
	"testing"
)

// TestPendingIsNotRawSourceContent guards a leak the docutils/rst
// v0.99.0 bump surfaced: ".. contents::" now parses to a <topic> whose
// only child is a <pending>, an INTERNAL node whose text is the parser's
// own ".. internal attributes:" block. parse.go already drops <pending>
// on the block path, but the raw-source reconstruction walked it as
// content and put ".transform: docutils.transforms.parts.Contents" into
// the rendered document.
func TestPendingIsNotRawSourceContent(t *testing.T) {
	d, err := Parse([]byte("x\n\n.. contents::\n\nT\n=\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Write(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, leak := range []string{"internal attributes", ".transform:", "docutils.transforms"} {
		if strings.Contains(string(out), leak) {
			t.Errorf("parser internals reached the output (%q):\n%s", leak, out)
		}
	}
	if !strings.Contains(string(out), ".. topic:: Contents") {
		t.Errorf("the topic itself was lost:\n%s", out)
	}
}
