package signal

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// newTestPairing builds a pairing store on a temporary home and a clock the test
// moves, which is what every rule below is measured against.
func newTestPairing(t *testing.T) (*Pairing, *testkit.FakeClock, contract.Home) {
	t.Helper()
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	pairing, err := NewPairing(home, clock)
	if err != nil {
		t.Fatalf("cannot open the pairing store: %v", err)
	}
	return pairing, clock, home
}

// offerTo asks for a code for one sender and fails the test when none is given.
func offerTo(t *testing.T, pairing *Pairing, sender string) string {
	t.Helper()
	code, offered, err := pairing.Offer(sender)
	if err != nil {
		t.Fatalf("offering a code to %s failed: %v", sender, err)
	}
	if !offered {
		t.Fatalf("no code was offered to %s, and one was due", sender)
	}
	return code
}

func TestPairingOffersACodeAndApprovesTheSenderWhoHasIt(t *testing.T) {
	pairing, _, _ := newTestPairing(t)

	if pairing.IsApproved("+15125550123") {
		t.Fatalf("a sender nobody approved is approved, and the list starts empty")
	}
	code := offerTo(t, pairing, "+15125550123")
	if _, valid := ReadPairingCode(code); !valid {
		t.Fatalf("the offered code %q is not a pairing code", code)
	}

	sender, err := pairing.Approve(code)
	if err != nil {
		t.Fatalf("approving the code failed: %v", err)
	}
	if sender != "+15125550123" {
		t.Errorf("the approved sender is %q, want the one the code was made for", sender)
	}
	if !pairing.IsApproved("+15125550123") {
		t.Errorf("the sender is still not approved after their code was approved")
	}
	if !pairing.HasApproved() {
		t.Errorf("the store says nobody is paired after one sender was paired")
	}
}

func TestPairingForgetsACodeAfterAnHour(t *testing.T) {
	pairing, clock, _ := newTestPairing(t)
	code := offerTo(t, pairing, "+15125550123")

	clock.Advance(PairingCodeLifetime + time.Minute)
	if _, err := pairing.Approve(code); err == nil {
		t.Fatalf("a code more than an hour old was approved, and a code lives one hour")
	}
	if pairing.IsApproved("+15125550123") {
		t.Errorf("the sender was approved by a code that had run out")
	}
}

func TestPairingHoldsOnlyTheCapAndANewSenderMakesRoom(t *testing.T) {
	pairing, clock, _ := newTestPairing(t)

	// The waiting list is filled to the cap, one sender every second, so that
	// which code has waited longest is known and none has run out of its hour.
	oldest := offerTo(t, pairing, "+15125550000")
	for number := 1; number < MaxPendingCodes; number++ {
		clock.Advance(time.Second)
		offerTo(t, pairing, fmt.Sprintf("+1512555%04d", number))
	}

	clock.Advance(time.Second)
	newest := offerTo(t, pairing, "+15125559999")
	if len(pairing.state.Pending) > MaxPendingCodes {
		t.Errorf("%d codes are waiting, and the cap is %d", len(pairing.state.Pending), MaxPendingCodes)
	}
	if _, err := pairing.Approve(oldest); err == nil {
		t.Errorf("the code that had waited longest still works, so nothing made room for the new sender")
	}
	sender, err := pairing.Approve(newest)
	if err != nil {
		t.Fatalf("the newest sender's code was refused: %v", err)
	}
	if sender != "+15125559999" {
		t.Errorf("the approved sender is %q, want the newest one", sender)
	}
}

func TestAWrongCodeLockoutStillOffersAnotherSenderACode(t *testing.T) {
	pairing, _, _ := newTestPairing(t)
	code := offerTo(t, pairing, "+15125550001")

	for try := range MaxWrongTries {
		if _, err := pairing.Approve("ZZZZ9999"); err == nil {
			t.Fatalf("wrong try %d was accepted", try+1)
		}
	}
	if _, err := pairing.Approve(code); err == nil {
		t.Fatalf("the right code was accepted after five wrong tries, and five wrong tries locks the door")
	}

	// The lockout is on typing codes, not on being handed one. The owner's own
	// phone must still be told how to pair, or one person's wrong codes shut
	// everybody out for an hour.
	if _, offered, err := pairing.Offer("+15125550123"); err != nil || !offered {
		t.Errorf("a sender was told nothing at all while the door was shut on somebody else's wrong codes: offered=%v err=%v", offered, err)
	}
}

func TestPairingAllowsOneRequestPerSenderEveryTenMinutes(t *testing.T) {
	pairing, clock, _ := newTestPairing(t)
	offerTo(t, pairing, "+15125550123")

	clock.Advance(PairingRequestInterval - time.Second)
	if _, offered, err := pairing.Offer("+15125550123"); err != nil || offered {
		t.Errorf("a second code was offered to the same sender within ten minutes (offered %v, error %v)", offered, err)
	}

	clock.Advance(2 * time.Second)
	if _, offered, err := pairing.Offer("+15125550123"); err != nil || !offered {
		t.Errorf("no code was offered to the same sender after ten minutes (offered %v, error %v)", offered, err)
	}
}

func TestPairingLocksOutAfterFiveWrongTriesAndOpensAgainAfterAnHour(t *testing.T) {
	pairing, clock, _ := newTestPairing(t)
	code := offerTo(t, pairing, "+15125550123")

	for try := range MaxWrongTries {
		if _, err := pairing.Approve("ZZZZ9999"); err == nil {
			t.Fatalf("wrong try %d was accepted", try+1)
		}
	}
	if _, err := pairing.Approve(code); err == nil {
		t.Fatalf("the right code was accepted after five wrong tries, and five wrong tries locks the door")
	}

	clock.Advance(PairingLockout + time.Minute)
	fresh := offerTo(t, pairing, "+15125550123")
	if _, err := pairing.Approve(fresh); err != nil {
		t.Errorf("a fresh code was still refused an hour after the lockout began: %v", err)
	}
}

func TestPairingFindsACodeWhereverItSitsInTheList(t *testing.T) {
	pairing, _, _ := newTestPairing(t)
	offerTo(t, pairing, "+15125550001")
	offerTo(t, pairing, "+15125550002")
	last := offerTo(t, pairing, "+15125550003")

	sender, err := pairing.Approve(last)
	if err != nil {
		t.Fatalf("approving the last of three codes failed, so the search stops early: %v", err)
	}
	if sender != "+15125550003" {
		t.Errorf("the approved sender is %q, want the third one", sender)
	}
}

func TestPairingStoresCodesSaltedAndHashedAndNeverInPlainWords(t *testing.T) {
	pairing, _, home := newTestPairing(t)
	first := offerTo(t, pairing, "+15125550001")

	written, err := os.ReadFile(home.SignalFolder() + "/pairing.json")
	if err != nil {
		t.Fatalf("cannot read the pairing file: %v", err)
	}
	if strings.Contains(string(written), first) {
		t.Errorf("the pairing file holds the code in plain words, and a code is kept salted and hashed")
	}

	sameSalt := saltsIn(t, string(written))
	if len(sameSalt) != 1 || len(sameSalt[0]) < 32 {
		t.Fatalf("the pairing file holds %d salts, want one of at least sixteen bytes", len(sameSalt))
	}

	info, err := os.Stat(home.SignalFolder() + "/pairing.json")
	if err != nil {
		t.Fatalf("cannot look at the pairing file: %v", err)
	}
	if info.Mode().Perm() != contract.SecretFileMode {
		t.Errorf("the pairing file is mode %v, want %v, because it holds secrets", info.Mode().Perm(), contract.SecretFileMode)
	}
}

func TestPairingRemembersApprovedSendersAcrossARestart(t *testing.T) {
	pairing, clock, home := newTestPairing(t)
	code := offerTo(t, pairing, "+15125550123")
	if _, err := pairing.Approve(code); err != nil {
		t.Fatalf("approving failed: %v", err)
	}

	reopened, err := NewPairing(home, clock)
	if err != nil {
		t.Fatalf("cannot open the pairing store again: %v", err)
	}
	if !reopened.IsApproved("+15125550123") {
		t.Errorf("the approved sender was forgotten when the store was opened again")
	}
}

func TestPairingMessageCarriesTheCodeAndWhatToDo(t *testing.T) {
	message := PairingMessage("ABCD2345")
	if !strings.Contains(message, "ABCD2345") {
		t.Errorf("the pairing message %q does not hold the code", message)
	}
	if !strings.Contains(message, "/pair") {
		t.Errorf("the pairing message %q does not say what to type", message)
	}
}

// saltsIn pulls the salt values out of the written pairing file, so that the
// test can see there is one and that it is long enough.
func saltsIn(t *testing.T, written string) []string {
	t.Helper()
	found := []string{}
	for _, part := range strings.Split(written, `"salt":"`)[1:] {
		end := strings.Index(part, `"`)
		if end < 0 {
			t.Fatalf("the pairing file has a salt with no end: %s", written)
		}
		found = append(found, part[:end])
	}
	return found
}
