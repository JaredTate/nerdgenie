// The pairing rules were borrowed from Hermes' pairing store at
// ~/Code/hermes-agent/gateway/pairing.py, which is where the hour a code lives,
// the cap on the codes waiting, the one request per sender every ten minutes,
// the five wrong tries before a lockout, and the salted hash come from, and from
// OpenClaw's pairing store at ~/Code/openclaw/src/pairing/pairing-store.ts,
// which is where the idea of consuming the pending request on approval comes
// from. The Go here is written fresh, and unlike either reference it compares
// the hashes in constant time.

package signal

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
	// PairingCodeLifetime is how long a code is good for.
	PairingCodeLifetime = time.Hour
	// MaxPendingCodes is how many senders may have a code waiting at once. One
	// sender holds one code, and a sender who arrives when the list is full
	// makes room by pushing out the code that has waited longest, so that no
	// number of strangers can hold every slot and leave the owner's own phone
	// with silence. The cap is the same as the number of senders the
	// ten-minute rule remembers, because the two lists hold the same people,
	// and it is what keeps a stream of strangers from filling the file.
	MaxPendingCodes = MaxRememberedRequests
	// PairingRequestInterval is how long one sender must wait between requests.
	PairingRequestInterval = 10 * time.Minute
	// MaxWrongTries is how many wrong codes are allowed before the door shuts.
	MaxWrongTries = 5
	// PairingLockout is how long the door stays shut after too many wrong tries.
	PairingLockout = time.Hour
	// MaxApprovedSenders is how many people may be paired at once, which keeps
	// the approved list something a person can still read.
	MaxApprovedSenders = 50
	// saltBytes is how many random bytes are mixed into a code before it is
	// hashed, so that two people who draw the same code store different hashes.
	saltBytes = 16
)

// errLockedOut says the door is shut after too many wrong codes.
var errLockedOut = errors.New("too many wrong pairing codes, so wait an hour and try again")

// errNoSuchCode says no waiting code matches.
var errNoSuchCode = errors.New("no sender is waiting with that pairing code, so ask them to send a message again for a fresh one")

// Pairing keeps the codes offered to senders the agent does not know and the
// list of senders it does, in two files under the home's Signal folder.
type Pairing struct {
	codesFile    string
	approvedFile string
	clock        contract.Clock

	guard    sync.Mutex
	state    pairingState
	approved approvedState
}

// NewPairing opens the pairing store under the home's Signal folder, reading
// what is already there. A file that is missing is an empty store; a file that
// cannot be read is an error, because carrying on would quietly unpair
// everybody.
func NewPairing(home contract.Home, clock contract.Clock) (*Pairing, error) {
	folder := home.SignalFolder()
	if err := makeFolder(folder); err != nil {
		return nil, err
	}
	pairing := &Pairing{
		codesFile:    codesFilePath(folder),
		approvedFile: approvedFilePath(folder),
		clock:        clock,
	}
	if err := readJSONFile(pairing.codesFile, &pairing.state); err != nil {
		return nil, err
	}
	if err := readJSONFile(pairing.approvedFile, &pairing.approved); err != nil {
		return nil, err
	}
	if pairing.state.LastRequest == nil {
		pairing.state.LastRequest = map[string]time.Time{}
	}
	return pairing, nil
}

// Offer gives one sender a fresh pairing code. The second result is false when
// the sender already asked within the last ten minutes, which is the only reason
// a sender is told nothing at all: what other senders have done is never a reason
// to leave this one in silence.
func (pairing *Pairing) Offer(sender string) (string, bool, error) {
	if sender == "" {
		return "", false, errors.New("cannot offer a pairing code to nobody, so pass the sender the daemon reported")
	}

	pairing.guard.Lock()
	defer pairing.guard.Unlock()
	now := pairing.clock.Now()
	pairing.forgetWhatHasRunOut(now)

	// The lockout after too many wrong codes is not looked at here. It shuts the
	// door on typing codes, which is where the guessing happens; a sender who is
	// handed a code guesses nothing, and refusing to hand one out would let one
	// person's wrong codes silence everybody's pairing for an hour.
	if asked, known := pairing.state.LastRequest[sender]; known && now.Sub(asked) < PairingRequestInterval {
		return "", false, nil
	}
	pairing.rememberRequest(sender, now)
	waiting := pairing.pendingIndexFor(sender)
	if waiting < 0 {
		pairing.makeRoomForOneMoreCode()
	}

	code, entry, err := newPendingCode(sender, now)
	if err != nil {
		return "", false, err
	}
	if waiting < 0 {
		pairing.state.Pending = append(pairing.state.Pending, entry)
	} else {
		pairing.state.Pending[waiting] = entry
	}
	if err := pairing.saveCodes(); err != nil {
		return "", false, err
	}
	return code, true, nil
}

// Approve pairs the sender whose waiting code matches the one typed. A code that
// matches nothing counts as a wrong try, and five wrong tries shut the door for
// an hour.
func (pairing *Pairing) Approve(typed string) (string, error) {
	code, valid := ReadPairingCode(typed)
	if !valid {
		return "", fmt.Errorf("%q is not a pairing code, so type the eight characters the sender was given", typed)
	}

	pairing.guard.Lock()
	defer pairing.guard.Unlock()
	now := pairing.clock.Now()
	pairing.forgetWhatHasRunOut(now)
	if now.Before(pairing.state.LockedUntil) {
		return "", errLockedOut
	}

	at := matchPendingCode(pairing.state.Pending, code)
	if at < 0 {
		return "", pairing.recordWrongTry(now)
	}
	sender := pairing.state.Pending[at].Sender
	pairing.state.Pending = append(pairing.state.Pending[:at], pairing.state.Pending[at+1:]...)
	pairing.state.WrongTries = 0
	if err := pairing.rememberApproved(sender, now); err != nil {
		return "", err
	}
	if err := pairing.saveCodes(); err != nil {
		return "", err
	}
	return sender, nil
}

// IsApproved says whether the agent has been told to listen to this sender.
func (pairing *Pairing) IsApproved(sender string) bool {
	pairing.guard.Lock()
	defer pairing.guard.Unlock()
	for _, one := range pairing.approved.Senders {
		if one.Sender == sender {
			return true
		}
	}
	return false
}

// HasApproved says whether anybody at all is paired, which is what decides
// whether the pairing command may be used from anywhere but the terminal.
func (pairing *Pairing) HasApproved() bool {
	pairing.guard.Lock()
	defer pairing.guard.Unlock()
	return len(pairing.approved.Senders) > 0
}

// PairingMessage is what an unknown sender is told, and the only thing they are
// told until somebody approves them.
func PairingMessage(code string) string {
	return "I do not know you yet, so I have not acted on your message.\n\n" +
		"Your pairing code is " + code + ".\n\n" +
		"Ask the person who runs this assistant to type \"/pair " + code +
		"\" in their terminal. The code is good for one hour."
}

// recordWrongTry counts one wrong code and shuts the door on the fifth. The
// caller holds the lock.
func (pairing *Pairing) recordWrongTry(now time.Time) error {
	pairing.state.WrongTries++
	if pairing.state.WrongTries >= MaxWrongTries {
		pairing.state.WrongTries = 0
		pairing.state.LockedUntil = now.Add(PairingLockout)
	}
	if err := pairing.saveCodes(); err != nil {
		return err
	}
	return errNoSuchCode
}

// rememberApproved adds the sender to the approved list and writes it out. The
// caller holds the lock.
func (pairing *Pairing) rememberApproved(sender string, now time.Time) error {
	for _, one := range pairing.approved.Senders {
		if one.Sender == sender {
			return nil
		}
	}
	if len(pairing.approved.Senders) >= MaxApprovedSenders {
		return fmt.Errorf("%d senders are already paired, which is the most this holds, so remove one before adding another", MaxApprovedSenders)
	}
	pairing.approved.Version = stateVersion
	pairing.approved.Senders = append(pairing.approved.Senders, approvedSender{Sender: sender, Approved: now})
	return writeJSONFile(pairing.approvedFile, pairing.approved)
}

// rememberRequest writes down when this sender last asked, keeping the list
// short so that a stream of strangers cannot grow it without end. The caller
// holds the lock.
func (pairing *Pairing) rememberRequest(sender string, now time.Time) {
	if len(pairing.state.LastRequest) >= MaxRememberedRequests {
		oldest, when := "", now
		for who, asked := range pairing.state.LastRequest {
			if !asked.After(when) {
				oldest, when = who, asked
			}
		}
		delete(pairing.state.LastRequest, oldest)
	}
	pairing.state.LastRequest[sender] = now
}

// makeRoomForOneMoreCode drops the codes that have waited longest until there is
// room for one more sender, so that a sender who arrives when the list is full is
// never the one turned away. Each pass drops one code, so the loop ends. The
// caller holds the lock.
func (pairing *Pairing) makeRoomForOneMoreCode() {
	for len(pairing.state.Pending) >= MaxPendingCodes {
		oldest := 0
		for at, entry := range pairing.state.Pending {
			if entry.Created.Before(pairing.state.Pending[oldest].Created) {
				oldest = at
			}
		}
		pairing.state.Pending = append(pairing.state.Pending[:oldest], pairing.state.Pending[oldest+1:]...)
	}
}

// forgetWhatHasRunOut drops codes older than an hour, requests older than the
// ten-minute wait, and a lockout whose hour is up. The caller holds the lock.
func (pairing *Pairing) forgetWhatHasRunOut(now time.Time) {
	kept := pairing.state.Pending[:0]
	for _, entry := range pairing.state.Pending {
		if now.Sub(entry.Created) < PairingCodeLifetime {
			kept = append(kept, entry)
		}
	}
	pairing.state.Pending = kept

	for who, asked := range pairing.state.LastRequest {
		if now.Sub(asked) >= PairingRequestInterval {
			delete(pairing.state.LastRequest, who)
		}
	}
	if !pairing.state.LockedUntil.IsZero() && !now.Before(pairing.state.LockedUntil) {
		pairing.state.LockedUntil = time.Time{}
		pairing.state.WrongTries = 0
	}
}

// pendingIndexFor finds the code already waiting for one sender, or minus one.
// The caller holds the lock.
func (pairing *Pairing) pendingIndexFor(sender string) int {
	for at, entry := range pairing.state.Pending {
		if entry.Sender == sender {
			return at
		}
	}
	return -1
}

// saveCodes writes the codes file. The caller holds the lock.
func (pairing *Pairing) saveCodes() error {
	pairing.state.Version = stateVersion
	return writeJSONFile(pairing.codesFile, pairing.state)
}

// newPendingCode draws a code, salts it, hashes it, and returns both the code to
// send and the entry to keep. The code itself is never written down.
func newPendingCode(sender string, now time.Time) (string, pendingCode, error) {
	code, err := newPairingCode()
	if err != nil {
		return "", pendingCode{}, err
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", pendingCode{}, fmt.Errorf("cannot salt a pairing code, because this machine gave no random numbers: %w", err)
	}
	entry := pendingCode{
		Sender:  sender,
		Salt:    hex.EncodeToString(salt),
		Hash:    hex.EncodeToString(hashPairingCode(salt, code)),
		Created: now,
	}
	return code, entry, nil
}

// matchPendingCode finds which waiting code the typed one is, or minus one. It
// looks at every entry and compares every hash in constant time, so that neither
// how long the answer takes nor where it stopped says anything about the code.
func matchPendingCode(pending []pendingCode, code string) int {
	found := -1
	for at, entry := range pending {
		salt, saltErr := hex.DecodeString(entry.Salt)
		want, hashErr := hex.DecodeString(entry.Hash)
		if saltErr != nil || hashErr != nil {
			continue
		}
		same := subtle.ConstantTimeCompare(hashPairingCode(salt, code), want)
		found = subtle.ConstantTimeSelect(same, at, found)
	}
	return found
}

// hashPairingCode mixes the salt into the code and hashes the two together.
func hashPairingCode(salt []byte, code string) []byte {
	material := make([]byte, 0, len(salt)+len(code))
	material = append(material, salt...)
	material = append(material, code...)
	sum := sha256.Sum256(material)
	return sum[:]
}
