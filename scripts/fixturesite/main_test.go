package main

import (
	"context"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestTheSiteComesUpOnTheAddressAndSaysWhatItAccepts starts the command on a
// port of the operating system's choosing, reads the address it prints, and
// fetches the login page from it.
func TestTheSiteComesUpOnTheAddressAndSaysWhatItAccepts(t *testing.T) {
	said, err := os.CreateTemp(t.TempDir(), "said")
	if err != nil {
		t.Fatalf("cannot make a file for what the command says: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, "127.0.0.1:0", testkit.FixturePagesFolder(), said) }()

	address := ""
	for waited := 0; waited < 50 && address == ""; waited++ {
		time.Sleep(20 * time.Millisecond)
		written, _ := os.ReadFile(said.Name())
		for _, line := range strings.Split(string(written), "\n") {
			if strings.HasPrefix(line, "the fixture site is at ") {
				address = strings.TrimPrefix(line, "the fixture site is at ")
			}
		}
	}
	if address == "" {
		t.Fatalf("the command never said where the site is")
	}
	answer, err := http.Get(address)
	if err != nil || answer.StatusCode != http.StatusOK {
		t.Fatalf("the login page did not answer at %s: %v", address, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("the command ended with %v, want a clean stop", err)
	}
}

// TestTheCommandRefusesAFolderWithNoPagesAndAnAddressItCannotListenOn covers
// the two ways the command can fail, each with the reason named.
func TestTheCommandRefusesAFolderWithNoPagesAndAnAddressItCannotListenOn(t *testing.T) {
	said, err := os.CreateTemp(t.TempDir(), "said")
	if err != nil {
		t.Fatalf("cannot make a file for what the command says: %v", err)
	}
	if err := run(context.Background(), "127.0.0.1:0", t.TempDir(), said); err == nil || !strings.Contains(err.Error(), "pages") {
		t.Errorf("a folder with no pages was not refused by name: %v", err)
	}
	if err := run(context.Background(), "127.0.0.1:1", testkit.FixturePagesFolder(), said); err == nil || !strings.Contains(err.Error(), "listen") {
		t.Errorf("an address the command cannot listen on was not refused by name: %v", err)
	}
}

// TestTheCommandLineNamesAFlagItDoesNotKnowAndABadAddress covers the command's
// own door: an unknown flag is refused with the usage code, and a bad address
// comes back as exit code one with the reason on the problems stream.
func TestTheCommandLineNamesAFlagItDoesNotKnowAndABadAddress(t *testing.T) {
	said, err := os.CreateTemp(t.TempDir(), "said")
	if err != nil {
		t.Fatalf("cannot make a file for what the command says: %v", err)
	}
	problems := &strings.Builder{}
	if code := command([]string{"--no-such-flag"}, said, problems); code != 2 {
		t.Errorf("an unknown flag came back as %d, want the usage code 2", code)
	}
	problems.Reset()
	if code := command([]string{"--listen", "127.0.0.1:1"}, said, problems); code != 1 || !strings.Contains(problems.String(), "listen") {
		t.Errorf("a bad address came back as %d saying %q, want 1 and the reason", code, problems.String())
	}
}
