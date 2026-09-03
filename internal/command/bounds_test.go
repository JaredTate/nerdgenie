package command_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theWordsAPersonWrote is what a test puts in a persona file to prove that
// "coeus init" never writes over what somebody wrote themselves.
const theWordsAPersonWrote = "# Who Coeus is\n\nShort answers. Never apologise. Say the cost.\n"

func TestTheWaitsAreTheOnesTheDesignAsksFor(t *testing.T) {
	for _, one := range []struct {
		what   string
		waited time.Duration
		wanted time.Duration
	}{
		{"one question", command.AnswerWait, 2 * time.Minute},
		{"the whole of coeus init", command.SetupWait, 20 * time.Minute},
		{"one call to the service manager", command.SystemctlWait, 30 * time.Second},
	} {
		if one.waited != one.wanted {
			t.Errorf("the wait on %s is %s rather than the %s the design asks for", one.what, one.waited, one.wanted)
		}
	}
}

func TestInitResetConfigLeavesAPersonaFileSomebodyWroteAlone(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	daemon := runningDaemon(t)
	setup := command.Setup{Home: home, Output: &strings.Builder{}, LocalAddress: daemon, LMStudioAddress: nothingListening(t)}

	if err := command.Init(context.Background(), setup, []string{"--model", "local", "--yes"}); err != nil {
		t.Fatalf("the first coeus init failed: %v", err)
	}
	if err := os.WriteFile(home.SoulFile(), []byte(theWordsAPersonWrote), contract.DataFileMode); err != nil {
		t.Fatalf("writing the hand-written persona file failed: %v", err)
	}

	if err := command.Init(context.Background(), setup, []string{"--model", "local", "--yes", "--reset-config"}); err != nil {
		t.Fatalf("coeus init --reset-config failed: %v", err)
	}

	after, err := os.ReadFile(home.SoulFile())
	if err != nil {
		t.Fatalf("reading the persona file back failed: %v", err)
	}
	if string(after) != theWordsAPersonWrote {
		t.Errorf("coeus init --reset-config wrote over what somebody wrote themselves:\n%s", after)
	}
}

func TestInitClosesAHomeFolderOtherAccountsCanLookInside(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	if err := os.MkdirAll(home.Root, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the home folder before coeus init ran failed: %v", err)
	}
	if err := os.Chmod(home.Root, 0o777); err != nil {
		t.Fatalf("opening the home folder to other accounts failed: %v", err)
	}

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    runningDaemon(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--model", "local", "--yes"})
	if err != nil {
		t.Fatalf("coeus init on a home folder that was already there failed: %v", err)
	}

	about, err := os.Stat(home.Root)
	if err != nil {
		t.Fatalf("looking at the home folder failed: %v", err)
	}
	if about.Mode().Perm() != contract.HomeFolderMode {
		t.Errorf("the home folder has mode %04o rather than %04o, so other accounts can still look inside it",
			about.Mode().Perm(), contract.HomeFolderMode)
	}
}

func TestOneQuestionIsAskedExactlyThreeTimes(t *testing.T) {
	for _, one := range []struct {
		what    string
		answers string
		works   bool
	}{
		{"two answers that will not do and then one that will", "\nbanana\npear\n1\n", true},
		{"three answers that will not do", "\nbanana\npear\nplum\n", false},
	} {
		t.Run(one.what, func(t *testing.T) {
			home := emptyHome(t)
			noProgramsOnThePath(t)

			err := command.Init(context.Background(), setupWithADaemon(t, home, one.answers, &strings.Builder{}), nil)
			if one.works && err != nil {
				t.Fatalf("coeus init gave up before the third try at the menu: %v", err)
			}
			if !one.works && err == nil {
				t.Fatalf("coeus init went on asking past the third try at the menu")
			}
		})
	}
}

// aNoisySystemctl puts a program called systemctl on the PATH that prints far
// more than anything is going to read and then refuses, so that a test can see
// whether what it printed is kept whole.
func aNoisySystemctl(t *testing.T, lines int) {
	t.Helper()
	folder := t.TempDir()
	script := "#!/bin/sh\n" +
		"count=0\n" +
		"while [ $count -lt " + strconv.Itoa(lines) + " ]; do\n" +
		"  echo 'the fake systemctl says a great deal and none of it matters' >&2\n" +
		"  count=$((count+1))\n" +
		"done\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(folder, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the noisy systemctl failed: %v", err)
	}
	t.Setenv("PATH", folder)
}

func TestWhatTheServiceManagerPrintedIsCappedBeforeItGoesInAMessage(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	aNoisySystemctl(t, 20000)

	err := aServiceIn(t, home, "", &strings.Builder{}).Install(context.Background())
	if err == nil {
		t.Fatalf("coeus install said nothing when the service manager refused")
	}
	if len(err.Error()) > command.MaxProgramOutputBytes*2 {
		t.Errorf("the message is %d bytes long, and what a program printed is capped at %d",
			len(err.Error()), command.MaxProgramOutputBytes)
	}
}
