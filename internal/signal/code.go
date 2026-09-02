// The alphabet and the length were borrowed from Hermes' pairing store at
// ~/Code/hermes-agent/gateway/pairing.py and from OpenClaw's pairing store at
// ~/Code/openclaw/src/pairing/pairing-store.ts, which both draw eight
// characters from the same thirty-two, leaving out the ones that look like
// each other. The Go here is written fresh.

package signal

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	// PairingCodeAlphabet is the thirty-two characters a pairing code is drawn
	// from. It is the letters and digits left over once the ones that look like
	// each other are taken out, so that a code read aloud or copied off a screen
	// comes back the same.
	PairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	// PairingCodeLength is how many characters a pairing code holds. Eight from
	// thirty-two is forty bits, which nobody guesses in the hour a code lives,
	// let alone in the five tries the store allows.
	PairingCodeLength = 8
	// maxTypedCodeLength is the most text the reader will look at before
	// refusing, so that a huge message cannot cost anything to reject.
	maxTypedCodeLength = 64
)

// newPairingCode draws one code at random from the alphabet. Two hundred and
// fifty-six divides evenly by the thirty-two characters of the alphabet, so
// taking the remainder of a random byte picks each character exactly as often as
// every other one. It fails only when the machine cannot give random numbers,
// which means the machine is broken.
func newPairingCode() (string, error) {
	drawn := make([]byte, PairingCodeLength)
	if _, err := rand.Read(drawn); err != nil {
		return "", fmt.Errorf("cannot draw a pairing code, because this machine gave no random numbers: %w", err)
	}
	for at, value := range drawn {
		drawn[at] = PairingCodeAlphabet[value%byte(len(PairingCodeAlphabet))]
	}
	return string(drawn), nil
}

// ReadPairingCode reads the code out of what a person typed after "/pair". It
// accepts small letters, spaces around the code, and a space or a dash in the
// middle, because that is how people write a code down. It returns false for
// anything that is not eight characters of the alphabet.
func ReadPairingCode(typed string) (string, bool) {
	if len(typed) > maxTypedCodeLength {
		return "", false
	}
	cleaned := strings.ToUpper(strings.TrimSpace(typed))
	cleaned = strings.Map(dropSpacersAndDashes, cleaned)
	if len(cleaned) != PairingCodeLength {
		return "", false
	}
	for _, letter := range cleaned {
		if !strings.ContainsRune(PairingCodeAlphabet, letter) {
			return "", false
		}
	}
	return cleaned, true
}

// dropSpacersAndDashes takes out the characters people put between the halves of
// a code and keeps everything else.
func dropSpacersAndDashes(letter rune) rune {
	switch letter {
	case ' ', '\t', '-', '_':
		return -1
	default:
		return letter
	}
}
