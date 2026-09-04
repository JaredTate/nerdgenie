package vault

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
)

// aVaultFilePath is the name every parse error must carry, so that the user
// knows which file to restore.
const aVaultFilePath = "/home/someone/.nerdgenie/vault.age"

func TestParsingAGoodDocumentGivesBackTheEntriesInNameOrder(t *testing.T) {
	plaintext := []byte(`{"version":1,"entries":[
		{"name":"x-account","site":"X","domains":["x.com"],"username":"jared","password":"a-long-password","totp_secret":"GEZDGNBVGY3TQOJQ"},
		{"name":"mail","site":"Fastmail","username":"someone","password":"another-long-password"}
	]}`)

	entries, err := parseDocument(aVaultFilePath, plaintext)
	if err != nil {
		t.Fatalf("parsing a good document failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("the document held %d entries, want 2", len(entries))
	}
	if entries[0].Name != "mail" || entries[1].Name != "x-account" {
		t.Errorf("the entries came back in the order %q and %q, want name order", entries[0].Name, entries[1].Name)
	}
	if entries[1].Password != "a-long-password" || entries[1].Domains[0] != "x.com" {
		t.Errorf("the entry x-account came back wrong")
	}
}

func TestEveryBadDocumentIsAnErrorThatNamesTheFile(t *testing.T) {
	tooMany, err := json.Marshal(storedDocument{Version: documentVersion, Entries: manyStoredEntries(MaxEntries + 1)})
	if err != nil {
		t.Fatalf("building the over-long document failed: %v", err)
	}

	bad := []struct {
		what      string
		plaintext []byte
	}{
		{"nothing at all", nil},
		{"text that is not JSON", []byte("this is not JSON")},
		{"a JSON list rather than an object", []byte(`[1,2,3]`)},
		{"a version this build does not write", []byte(`{"version":99,"entries":[]}`)},
		{"no version at all", []byte(`{"entries":[]}`)},
		{"an entry with no name", []byte(`{"version":1,"entries":[{"site":"X"}]}`)},
		{"an entry whose name holds a space", []byte(`{"version":1,"entries":[{"name":"two words"}]}`)},
		{"two entries with the same name", []byte(`{"version":1,"entries":[{"name":"mail"},{"name":"mail"}]}`)},
		{"an entry with a domain that is not a hostname", []byte(`{"version":1,"entries":[{"name":"mail","domains":["a b/c"]}]}`)},
		{"more entries than the limit", tooMany},
	}

	for _, one := range bad {
		entries, err := parseDocument(aVaultFilePath, one.plaintext)
		if err == nil {
			t.Errorf("a document holding %s was parsed as though it were good", one.what)
			continue
		}
		if !strings.Contains(err.Error(), aVaultFilePath) {
			t.Errorf("the error for %s is %q and does not name the file", one.what, err)
		}
		if entries != nil {
			t.Errorf("a document holding %s gave back entries as well as an error", one.what)
		}
	}
}

// manyStoredEntries builds a document with more entries than the limit allows.
func manyStoredEntries(count int) []storedEntry {
	entries := make([]storedEntry, 0, count)
	for at := 0; at < count; at++ {
		entries = append(entries, storedEntry{Name: fmt.Sprintf("login-%d", at)})
	}
	return entries
}

func TestAnEntryWithAValueOverTheLimitIsRefused(t *testing.T) {
	err := checkEntry(Entry{Name: "mail", Password: strings.Repeat("a", MaxValueLength+1)})
	if err == nil {
		t.Fatalf("an entry with a value over the limit was accepted")
	}
	if !strings.Contains(err.Error(), "mail") {
		t.Errorf("the error %q does not name the entry", err)
	}

	if err := checkEntry(Entry{Name: strings.Repeat("n", MaxNameLength+1)}); err == nil {
		t.Errorf("an entry with a name over the limit was accepted")
	}
	if err := checkEntry(Entry{Name: "mail", Domains: make([]string, MaxDomains+1)}); err == nil {
		t.Errorf("an entry naming more domains than the limit was accepted")
	}
}

func TestAKeyFileIsSafeOnlyWhenNobodyElseCanReadItAndItIsOurs(t *testing.T) {
	const path = "/home/someone/.nerdgenie/vault.key"
	const ourUser = 1000

	if err := checkKeyFileSafety(path, 0o600, ourUser, ourUser); err != nil {
		t.Errorf("a key file of mode 0600 owned by us was refused: %v", err)
	}
	if err := checkKeyFileSafety(path, 0o400, ourUser, ourUser); err != nil {
		t.Errorf("a key file of mode 0400 owned by us was refused: %v", err)
	}

	for _, mode := range []fs.FileMode{0o640, 0o604, 0o644, 0o660, 0o666, 0o777} {
		err := checkKeyFileSafety(path, mode, ourUser, ourUser)
		if err == nil {
			t.Errorf("a key file of mode %#o was accepted, and others can read it", mode)
			continue
		}
		if !strings.Contains(err.Error(), "0600") || !strings.Contains(err.Error(), path) {
			t.Errorf("the error for mode %#o is %q and must name the file and the mode to set", mode, err)
		}
	}

	err := checkKeyFileSafety(path, 0o600, 0, ourUser)
	if err == nil {
		t.Fatalf("a key file owned by another account was accepted")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error %q does not name the file", err)
	}
}

func TestTheDomainsAnswerIsSplitOnCommasAndEmptyWhenThereAreNone(t *testing.T) {
	cases := []struct {
		answer  string
		domains []string
	}{
		{"", nil},
		{" , ,, ", nil},
		{"x.com", []string{"x.com"}},
		{" x.com , twitter.com ", []string{"x.com", "twitter.com"}},
	}
	for _, one := range cases {
		if got := splitDomains(one.answer); !slices.Equal(got, one.domains) {
			t.Errorf("the answer %q became the domains %v, want %v", one.answer, got, one.domains)
		}
	}
}

func FuzzParseDocumentNeverPanicsAndAlwaysNamesTheFile(f *testing.F) {
	seeds := [][]byte{
		nil,
		[]byte(""),
		[]byte("{}"),
		[]byte(`{"version":1,"entries":[]}`),
		[]byte(`{"version":1,"entries":[{"name":"mail","password":"a-long-password"}]}`),
		[]byte(`{"version":1,"entries":[{"name":"","site":"X"}]}`),
		[]byte(`{"version":1,"entries":[{"name":"mail"},{"name":"mail"}]}`),
		[]byte("\x00\xff not utf-8 and not JSON"),
		[]byte(strings.Repeat(`{"version":1,`, 200)),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		entries, err := parseDocument(aVaultFilePath, plaintext)
		if err != nil {
			if !strings.Contains(err.Error(), aVaultFilePath) {
				t.Errorf("the error %q does not name the file that could not be read", err)
			}
			if entries != nil {
				t.Errorf("a document that failed to parse still gave back %d entries", len(entries))
			}
			return
		}
		checkTheEntriesSurviveBeingWrittenAgain(t, entries)
	})
}

// checkTheEntriesSurviveBeingWrittenAgain writes what was parsed back out and
// parses it once more, because anything the parser accepts must be something
// the writer can write.
func checkTheEntriesSurviveBeingWrittenAgain(t *testing.T, entries []Entry) {
	t.Helper()
	written, err := serializeDocument(entries)
	if err != nil {
		t.Fatalf("entries that parsed could not be written again: %v", err)
	}
	again, err := parseDocument(aVaultFilePath, written)
	if err != nil {
		t.Fatalf("entries that parsed could not be parsed again: %v", err)
	}
	if len(again) != len(entries) {
		t.Fatalf("writing and parsing again gave %d entries, want %d", len(again), len(entries))
	}
	for at := range entries {
		if again[at].Name != entries[at].Name || again[at].Password != entries[at].Password {
			t.Errorf("the entry at %d changed when it was written and parsed again", at)
		}
		if !slices.Equal(again[at].Domains, entries[at].Domains) {
			t.Errorf("the domains of the entry at %d changed when it was written and parsed again", at)
		}
	}
}
