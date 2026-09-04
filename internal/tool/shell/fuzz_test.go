package shell_test

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/shell"
)

// FuzzTheCommandReader throws any text at the reader of a shell call's arguments
// and asserts the two things that must always hold: it never panics, and a call
// it accepts is one this tool could actually run.
func FuzzTheCommandReader(f *testing.F) {
	f.Add(`{"command":"echo alpha"}`)
	f.Add(`{"action":"poll","id":"p1"}`)
	f.Add(`{"command":"apt install nginx","escalate":true,"reason":"the package is missing"}`)
	f.Add(`{"command":`)
	f.Add(``)
	f.Add("{\"command\":\"\\u0000\"}")

	f.Fuzz(func(t *testing.T, written string) {
		if len(written) > 1<<16 {
			t.Skip("the fuzzer wrote more text than one call ever carries")
		}
		asked, err := shell.ReadCall(context.Background(), []byte(written))
		if err != nil {
			return
		}
		if !shell.KnownAction(asked.Action) {
			t.Fatalf("the reader accepted the action %q, which the tool does not know", asked.Action)
		}
		if asked.Action == shell.ActionRun && asked.Command == "" {
			t.Fatalf("the reader accepted a run with no command in it")
		}
		if asked.Action != shell.ActionRun && asked.ID == "" {
			t.Fatalf("the reader accepted a %s with no id in it", asked.Action)
		}
		if len(asked.Command) > shell.MaxCommandBytes {
			t.Fatalf("the reader accepted a command of %d bytes, and the cap is %d",
				len(asked.Command), shell.MaxCommandBytes)
		}
	})
}
