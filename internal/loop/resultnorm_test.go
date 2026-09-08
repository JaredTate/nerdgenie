package loop

import "testing"

// TestVolatileTokensAreStrippedButMeaningKept: two results that differ only in
// a clock time, a process id, a port or a temporary path read as the same
// result, so a wall hit twice reads as one wall; two that differ in a
// count or a message do not, so real progress — twelve failing becoming eleven
// — is never hidden. The second half is the safety half: over-stripping would
// merge a converging check into a frozen one and call moving work a wall.
func TestVolatileTokensAreStrippedButMeaningKept(t *testing.T) {
	same := []struct{ one, two string }{
		{"the command timed out at 12:14:07, killed pid 900", "the command timed out at 12:28:31, killed pid 4102"},
		{"launched Chrome on the loopback DevTools port 38645", "launched Chrome on the loopback DevTools port 39491"},
		{"wrote /tmp/claude-1/probe.js", "wrote /tmp/claude-9/other.js"},
		{"panic at 0xdeadBEEF", "panic at 0x00c0004"},
	}
	for _, pair := range same {
		if normalizedFingerprint(pair.one) != normalizedFingerprint(pair.two) {
			t.Errorf("these should read as the same wall but did not:\n  %q\n  %q", pair.one, pair.two)
		}
	}
	different := []struct{ one, two string }{
		{"12 failing of 42", "11 failing of 42"}, // a count is meaning, not noise
		{"12 failing of 42", "12 failing of 43"},
		{"cannot find module logic", "cannot find module server"},
		{"all 20 tests passing", "all 30 tests passing"},
		{"exit code 0", "exit code 1"},
		// A duration is left in on purpose: two runs of a stuck test that
		// differ only by their milliseconds must stay two results, so the
		// test-state tracker's own stuck line shows before the wall fires.
		{"1 test failing (0.5ms)", "1 test failing (3.5ms)"},
		// A compiler's line and column must stay in: two different errors in
		// one file must read as two results, not one wall.
		{"main.go:12:34: undefined name", "main.go:56:78: undefined name"},
		// An ordinary H:MM time or score is not a timestamp to strip.
		{"the meeting is at 12:14", "the meeting is at 9:45"},
	}
	for _, pair := range different {
		if normalizedFingerprint(pair.one) == normalizedFingerprint(pair.two) {
			t.Errorf("these are different results but read as the same wall:\n  %q\n  %q", pair.one, pair.two)
		}
	}
}

// FuzzNormalizeVolatile: the normalizer parses text from outside, so it is
// fuzzed. Whatever bytes it is given, it never panics and never grows the
// text, which is all a fingerprint's input has to promise.
func FuzzNormalizeVolatile(f *testing.F) {
	for _, seed := range []string{"", "12:14:07", "in 3.2s pid 40", "12 of 42", "/tmp/x 0xdeadBEEF", "port 8090"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		// The two invariants a fingerprint's input must promise, over any
		// bytes: normalizing never panics, and never grows the text (every
		// token is replaced by a single space and every token is longer than
		// one letter). A result is normalized exactly once when it is noted,
		// so a second pass is never taken and idempotence is not required; the
		// meaning-preserving property — a count or a message is never merged
		// away — is pinned by the unit test above instead.
		out := normalizeVolatile(text)
		if len(out) > len(text) {
			t.Fatalf("normalizing grew the text from %d to %d bytes", len(text), len(out))
		}
	})
}
