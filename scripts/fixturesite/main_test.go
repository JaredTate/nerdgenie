package main

import (
	"context"
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
	go func() { done <- run(ctx, "127.0.0.1:0", said) }()

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
