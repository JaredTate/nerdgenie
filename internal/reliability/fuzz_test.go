package reliability_test

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/reliability"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// FuzzTheArchiveReader throws arbitrary bytes at the restore, both as the
// archive itself and as the contents inside a properly locked archive. Every
// one of them has to come back as a restore that worked or as an error naming
// the archive, never as a panic and never as a file written outside the home
// folder, because an archive is a file from outside the program and the restore
// is the one place bytes from outside decide what is written where.
func FuzzTheArchiveReader(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("age-encryption.org/v1\n"))
	f.Add([]byte{0x00, 0x01, 0xff, 0xfe})
	f.Add(tarHolding("vault.age", "the encrypted vault"))
	f.Add(tarHolding("browser/default/Preferences", "what the browser remembers"))
	f.Add(tarHolding("../../etc/passwd", "somebody else's file"))
	f.Add(tarHolding("/etc/passwd", "somebody else's file"))
	f.Add(tarHolding("browser/../../escaped", "somebody else's file"))
	f.Add(tarHolding("coeus.db", "not really a database"))

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		f.Fatalf("making the age key failed: %v", err)
	}

	f.Fuzz(func(t *testing.T, written []byte) {
		// The folder is made fresh for every input, so that what one input
		// unpacked cannot slow the next one down.
		folder := t.TempDir()
		home := contract.NewHome(filepath.Join(folder, "home", ".coeus"))
		keyFile := filepath.Join(folder, "vault.key")
		archive := filepath.Join(folder, "archive.tar.age")
		if err := os.WriteFile(keyFile, []byte(identity.String()+"\n"), contract.SecretFileMode); err != nil {
			t.Fatalf("writing the key file failed: %v", err)
		}

		for _, bytes := range [][]byte{written, locked(t, written, identity.Recipient())} {
			if err := os.WriteFile(archive, bytes, contract.SecretFileMode); err != nil {
				t.Fatalf("writing the archive failed: %v", err)
			}
			err := reliability.Restore(context.Background(), reliability.RestoreSettings{
				Home:    home,
				Archive: archive,
				KeyFile: keyFile,
				Force:   true,
			})
			if err != nil && !strings.Contains(err.Error(), archive) {
				t.Fatalf("the restore failed without naming the archive: %v", err)
			}
			nothingEscapedTheHome(t, folder)
		}
	})
}

// FuzzTheMarkerFilesUnderTheRunFolder throws arbitrary bytes at the two small
// files the breaker and the drain marker keep their state in, because both are
// read at startup and a program that cannot start is a program that cannot be
// fixed from a phone.
func FuzzTheMarkerFilesUnderTheRunFolder(f *testing.F) {
	f.Add([]byte(`{"starts":["2026-09-02T03:00:00Z"],"trippedAt":"0001-01-01T00:00:00Z"}`))
	f.Add([]byte(`{"bootID":"a boot","requestedAt":"2026-09-02T03:00:00Z"}`))
	f.Add([]byte(`{"starts":`))
	f.Add([]byte("null"))
	f.Add([]byte{0xff, 0x00})

	f.Fuzz(func(t *testing.T, written []byte) {
		home := contract.NewHome(t.TempDir())
		if err := os.MkdirAll(home.RunFolder(), contract.HomeFolderMode); err != nil {
			t.Fatalf("making the run folder failed: %v", err)
		}
		clock := testkit.NewFakeClock(startOfTime)
		for _, name := range []string{reliability.BreakerFileName, reliability.DrainFileName} {
			if err := os.WriteFile(filepath.Join(home.RunFolder(), name), written, contract.DataFileMode); err != nil {
				t.Fatalf("writing %s failed: %v", name, err)
			}
		}

		// A file the breaker cannot read must leave it open, because a breaker
		// that trips on damage stops a healthy agent from working.
		if tripped, err := reliability.NewBreaker(home, clock).Tripped(); tripped && err != nil {
			t.Fatalf("a breaker file that could not be read tripped the breaker anyway: %v", err)
		}
		reliability.NewDrain(home, clock).Requested()
		if _, err := reliability.PrepareDatabase(context.Background(), reliability.RecoverySettings{Home: home, Clock: clock}); err != nil {
			t.Fatalf("preparing the database over these marker files failed: %v", err)
		}
	})
}

// tarHolding builds a tar file with one file in it, which is what an archive
// holds once it is unlocked.
func tarHolding(name string, written string) []byte {
	held := &bytes.Buffer{}
	archive := tar.NewWriter(held)
	header := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o600, Size: int64(len(written))}
	if archive.WriteHeader(header) != nil || archive.Close() != nil {
		return held.Bytes()
	}
	return held.Bytes()
}

// locked puts the bytes inside a properly encrypted archive, so that the fuzz
// reaches the reader that unpacks one rather than stopping at the lock.
func locked(t *testing.T, written []byte, recipient age.Recipient) []byte {
	t.Helper()
	held := &bytes.Buffer{}
	sealing, err := age.Encrypt(held, recipient)
	if err != nil {
		t.Fatalf("locking the archive failed: %v", err)
	}
	if _, err := sealing.Write(written); err != nil {
		t.Fatalf("writing into the locked archive failed: %v", err)
	}
	if err := sealing.Close(); err != nil {
		t.Fatalf("sealing the archive failed: %v", err)
	}
	return held.Bytes()
}

// nothingEscapedTheHome fails the test when the restore wrote anything outside
// the home folder it was given.
func nothingEscapedTheHome(t *testing.T, folder string) {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("reading the folder the fuzz works in failed: %v", err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "home", "vault.key", "archive.tar.age":
		default:
			t.Fatalf("the restore wrote %q outside the home folder it was given", entry.Name())
		}
	}
}
