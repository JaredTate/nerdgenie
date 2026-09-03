package edit_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/edit"
)

// FuzzTheMatchers throws any three pieces of text at the matcher and asserts the
// two things that must always hold: it never panics, and an edit it accepts
// really did put the new text in and is no longer than the text plus the change.
func FuzzTheMatchers(f *testing.F) {
	f.Add("alpha\nbeta\n", "beta", "delta")
	f.Add("let  x   =  1\n", "let x = 1", "let x = 2")
	f.Add("    if ok {\n        return 1\n    }\n", "if ok {\n    return 1\n}", "return 2")
	f.Add("beta\nbeta\n", "beta", "delta")
	f.Add("", "", "")
	f.Add("\x00\x00", "\x00", "\n")

	f.Fuzz(func(t *testing.T, before string, old string, replacement string) {
		if len(before) > 1<<16 || len(old) > 1<<16 || len(replacement) > 1<<16 {
			t.Skip("the fuzzer wrote more text than one edit ever carries")
		}
		after, how, err := edit.Replace(before, old, replacement)
		if err != nil {
			if after != "" {
				t.Fatalf("a refused edit still handed back %d bytes of text", len(after))
			}
			return
		}
		if how == "" {
			t.Fatalf("an edit was made and no matcher was named for it")
		}
		if !strings.Contains(after, replacement) {
			t.Fatalf("an edit was made and the new text is not in what came back")
		}
		if len(after) > len(before)+len(replacement) {
			t.Fatalf("an edit turned %d bytes into %d, which is more than the text plus the change",
				len(before), len(after))
		}
	})
}
