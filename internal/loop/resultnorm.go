package loop

import (
	"crypto/sha256"
	"fmt"
	"regexp"
)

// normalizedFingerprint is one result in a fixed number of letters, with the
// tokens that change between two otherwise identical results taken out first —
// a clock time, a process id, a port, a temporary path, an address — so that
// the same wall, hit twice, reads as the same wall. It is
// deliberately conservative: it strips only tokens that carry no meaning of
// their own, and never a count or a message, because merging a check that is
// converging into one that is frozen would call moving work a wall. The
// same-wall detector (samewall.go) is the only reader; the guard keeps its own
// exact fingerprint (fingerprintOfText), which must not change.
func normalizedFingerprint(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(normalizeVolatile(text))))
}

// volatilePatterns are the tokens normalizeVolatile takes out. Each matches a
// structured, meaningless-on-its-own token; none matches a bare number, so a
// test count, an exit code or a byte size is left untouched. The clock pattern
// requires the seconds field, so that a compiler's "file.go:12:34" line and
// column, or a score or a price written H:MM, is never mistaken for a
// timestamp and merged with a different one — merging two different results
// would inflate the count and call distinct errors one wall. The port pattern
// is anchored to the browser worker's own line for the same reason. Durations
// are deliberately left in: the test-state tracker (teststate.go) already reads
// a run's failing set rather than its bytes so that a changing duration does
// not hide a stuck test, and taking durations out here would make this detector
// fire on the same stuck run before that tracker's own line ever showed.
var volatilePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b\d{1,2}:\d{2}:\d{2}(\.\d+)?\b`),    // clock times, seconds required: 12:14:07
	regexp.MustCompile(`\b(pid|process id|process)\s+\d+\b`), // process ids: "pid 900"
	regexp.MustCompile(`\bDevTools port \d+\b`),              // the browser worker's DevTools line
	regexp.MustCompile(`0x[0-9a-fA-F]{3,}`),                  // addresses: 0xdeadBEEF
	regexp.MustCompile(`/tmp/\S+`),                           // temporary paths
}

// normalizeVolatile takes the volatile tokens out of a result, one pattern at
// a time, so that the fingerprint over what is left is stable across two runs
// that differ only in when they ran or where their scratch went. Each token is
// replaced with a single space, not nothing, so that removing it cannot join
// its neighbours into one word ("cat" and "dog" around a stripped token stay
// two words, not "catdog"); every token is at least five letters, so the text
// never grows. A result is normalized once when it is noted, so the passes are
// applied once and left there.
func normalizeVolatile(text string) string {
	for _, pattern := range volatilePatterns {
		text = pattern.ReplaceAllString(text, " ")
	}
	return text
}
