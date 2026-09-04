package command_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// answersThatCannotBeRead is an input that fails rather than ending, which is
// what a terminal closed while a question was waiting looks like.
type answersThatCannotBeRead struct{}

// Read always fails, so that the answer reader takes its failing path.
func (answersThatCannotBeRead) Read([]byte) (int, error) {
	return 0, errors.New("the terminal was closed while a question was waiting")
}

// aGivingUpRun is one of the ways "coeus init" gives up on a question, as the
// name of the way and the context and setup that bring it about.
type aGivingUpRun struct {
	what  string
	build func(t *testing.T, home contract.Home) (context.Context, command.Setup)
}

// givingUpRuns are the six ways "coeus init" stops part way through. Every one
// of them leaves a machine with a home folder, the folders inside it, and the
// persona files, and with no config.toml, so none of them may say that nothing
// was set up.
func givingUpRuns() []aGivingUpRun {
	return []aGivingUpRun{{
		what: "a menu nobody answers with a number",
		build: func(t *testing.T, home contract.Home) (context.Context, command.Setup) {
			noProgramsOnThePath(t)
			return context.Background(), setupWithADaemon(t, home, "\nbanana\npear\nplum\n", &strings.Builder{})
		},
	}, {
		what: "a folder nobody names properly",
		build: func(t *testing.T, home contract.Home) (context.Context, command.Setup) {
			noProgramsOnThePath(t)
			userHome, err := os.UserHomeDir()
			if err != nil {
				t.Fatalf("reading the home directory failed: %v", err)
			}
			answers := strings.Repeat(userHome+"\n", maxAnswersInATest)
			return context.Background(), setupWithADaemon(t, home, answers, &strings.Builder{})
		},
	}, {
		what: "a yes-or-no question nobody answers yes or no",
		build: func(t *testing.T, home contract.Home) (context.Context, command.Setup) {
			putProgramOnThePath(t, noProgramsOnThePath(t), "signal-cli")
			return context.Background(), setupWithADaemon(t, home, "\n1\nmaybe\nmaybe\nmaybe\n", &strings.Builder{})
		},
	}, {
		what: "answers that run out before the questions do",
		build: func(t *testing.T, home contract.Home) (context.Context, command.Setup) {
			noProgramsOnThePath(t)
			return context.Background(), setupWithADaemon(t, home, "\n", &strings.Builder{})
		},
	}, {
		what: "an input that cannot be read at all",
		build: func(t *testing.T, home contract.Home) (context.Context, command.Setup) {
			noProgramsOnThePath(t)
			setup := setupWithADaemon(t, home, "", &strings.Builder{})
			setup.Input = answersThatCannotBeRead{}
			return context.Background(), setup
		},
	}, {
		what: "no answer arriving at all",
		build: func(t *testing.T, home contract.Home) (context.Context, command.Setup) {
			noProgramsOnThePath(t)
			neverAnswers, _ := io.Pipe()
			t.Cleanup(func() { _ = neverAnswers.Close() })
			setup := setupWithADaemon(t, home, "", &strings.Builder{})
			setup.Input = neverAnswers

			stopped, stop := context.WithCancel(context.Background())
			stop()
			return stopped, setup
		},
	}}
}

func TestInitNeverSaysNothingWasSetUpOnceTheHomeFolderIsThere(t *testing.T) {
	for _, one := range givingUpRuns() {
		t.Run(one.what, func(t *testing.T) {
			home := emptyHome(t)
			ctx, setup := one.build(t, home)

			err := command.Init(ctx, setup, nil)
			if err == nil {
				t.Fatalf("coeus init carried on past %s", one.what)
			}

			if strings.Contains(err.Error(), "nothing was set up") {
				t.Errorf("coeus init says nothing was set up, and the home folder is already there: %v", err)
			}
			for _, wanted := range []string{"home folder was made", "no configuration was written", "coeus init again"} {
				if !strings.Contains(err.Error(), wanted) {
					t.Errorf("the message leaves out %q, so it does not say where a second run picks up: %v", wanted, err)
				}
			}
			assertTheHomeIsThereWithNoConfiguration(t, home)
		})
	}
}

// assertTheHomeIsThereWithNoConfiguration proves that what the giving-up
// message says is true: the layout and the persona files are on disk and
// config.toml is not.
func assertTheHomeIsThereWithNoConfiguration(t *testing.T, home contract.Home) {
	t.Helper()
	for _, path := range append(home.Folders(), home.SoulFile(), home.UserFactsFile(), home.WorldFactsFile()) {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s is not there, so the message about the home folder would have been wrong: %v", path, err)
		}
	}
	if _, err := os.Stat(home.ConfigFile()); !os.IsNotExist(err) {
		t.Errorf("a configuration was written even though coeus init gave up: %v", err)
	}
}
