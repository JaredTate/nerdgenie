package signal

import (
	"strings"
	"testing"
)

func TestPairingCodeAlphabetHasThirtyTwoCharactersAndNoLookAlikes(t *testing.T) {
	if len(PairingCodeAlphabet) != 32 {
		t.Errorf("the alphabet holds %d characters, want thirty-two", len(PairingCodeAlphabet))
	}
	for _, lookAlike := range "01IO" {
		if strings.ContainsRune(PairingCodeAlphabet, lookAlike) {
			t.Errorf("the alphabet holds %q, and a character that looks like another one gets read back wrong over the phone", lookAlike)
		}
	}
	seen := map[rune]bool{}
	for _, letter := range PairingCodeAlphabet {
		if seen[letter] {
			t.Errorf("the alphabet holds %q twice, which wastes a character of the code", letter)
		}
		seen[letter] = true
	}
}

func TestNewPairingCodeIsEightCharactersFromTheAlphabetAndDiffersEachTime(t *testing.T) {
	made := map[string]bool{}
	for range 200 {
		code, err := newPairingCode()
		if err != nil {
			t.Fatalf("making a pairing code failed: %v", err)
		}
		if len(code) != PairingCodeLength {
			t.Fatalf("the code %q is %d characters, want %d", code, len(code), PairingCodeLength)
		}
		for _, letter := range code {
			if !strings.ContainsRune(PairingCodeAlphabet, letter) {
				t.Fatalf("the code %q holds %q, which is not in the alphabet", code, letter)
			}
		}
		made[code] = true
	}
	if len(made) < 190 {
		t.Errorf("two hundred codes gave only %d different ones, so the codes are not being drawn at random", len(made))
	}
}

func TestReadPairingCodeAcceptsWhatAPersonWouldType(t *testing.T) {
	cases := []struct {
		name  string
		typed string
		want  string
	}{
		{"exactly right", "ABCD2345", "ABCD2345"},
		{"in small letters", "abcd2345", "ABCD2345"},
		{"with spaces around it", "  ABCD2345\n", "ABCD2345"},
		{"written in two halves", "ABCD 2345", "ABCD2345"},
		{"written with a dash", "ABCD-2345", "ABCD2345"},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			code, ok := ReadPairingCode(oneCase.typed)
			if !ok {
				t.Fatalf("the reader refused %q, which is a code %s", oneCase.typed, oneCase.name)
			}
			if code != oneCase.want {
				t.Errorf("the reader made %q into %q, want %q", oneCase.typed, code, oneCase.want)
			}
		})
	}
}

func TestReadPairingCodeRefusesWhatIsNotACode(t *testing.T) {
	cases := []struct {
		name  string
		typed string
	}{
		{"nothing", ""},
		{"only spaces", "   "},
		{"too short", "ABCD234"},
		{"too long", "ABCD23456"},
		{"a character that is not in the alphabet", "ABCD234!"},
		{"a look-alike character", "ABCD2340"},
		{"a whole sentence", "please let me in"},
		{"far too much text", strings.Repeat("A", 10000)},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			if code, ok := ReadPairingCode(oneCase.typed); ok {
				t.Errorf("the reader read %s as the code %q, and only an eight-character code is one", oneCase.name, code)
			}
		})
	}
}
