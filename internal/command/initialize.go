package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
)

// Setup is everything "coeus init" needs from the outside world, so that a test
// can drive the whole thing with no terminal, no daemon, and no real home
// folder.
type Setup struct {
	// Home is the folder to set up.
	Home contract.Home
	// Input is where the answers are read from. A nil input asks nothing and
	// takes every default, which is what a container gets.
	Input io.Reader
	// Output is where the questions, the doctor's report, and the closing lines
	// are printed.
	Output io.Writer
	// AskSecret reads an API key without echoing it. The running program passes
	// the vault's masked prompt; a test passes its own.
	AskSecret func(prompt string) (string, error)
	// LocalAddress is the base address of the local model daemon, and is the
	// address the shipped configuration names when it is empty.
	LocalAddress string
	// LMStudioAddress is the base address an LM Studio server answers at, and is
	// LMStudioAddress when it is empty.
	LMStudioAddress string
}

// localAddress is where the local model daemon is looked for.
func (setup Setup) localAddress() string {
	if setup.LocalAddress != "" {
		return setup.LocalAddress
	}
	return contract.DefaultConfig().Models[0].BaseAddress
}

// lmStudioAddress is where an LM Studio server is looked for.
func (setup Setup) lmStudioAddress() string {
	if setup.LMStudioAddress != "" {
		return setup.LMStudioAddress
	}
	return LMStudioAddress
}

// Init sets Coeus up on a machine that has never run it: it makes the home
// folder and the work folder, writes the three persona files, asks at most six
// questions, writes config.toml, runs the doctor, and prints the commands a new
// user needs.
//
// A home folder that already has a configuration is left exactly as it is,
// because a second "coeus init" is nearly always a mistake; --reset-config
// writes the configuration again on purpose.
func Init(ctx context.Context, setup Setup, arguments []string) error {
	if setup.Output == nil {
		return errors.New("coeus init has nowhere to print its questions, so give the setup an output writer")
	}
	chosen, err := readInitFlags(arguments, setup.Output)
	if err != nil {
		return err
	}

	if _, err := os.Stat(setup.Home.ConfigFile()); err == nil && !chosen.resetConfig {
		fmt.Fprintf(setup.Output, "\n%s is already set up, so nothing was changed.\n", setup.Home.Root)
		fmt.Fprintf(setup.Output, "Run \"coeus init --reset-config\" to write %s again.\n", setup.Home.ConfigFile())
		return setup.finish(ctx, false)
	}
	return setup.run(ctx, chosen)
}

// run is the setup itself, in the order a person answers it: the folders to
// work in, the model, the key when the model needs one, and Signal.
func (setup Setup) run(ctx context.Context, chosen initFlags) error {
	ask := newAsker(setup.Input, setup.Output, chosen.yes)
	fmt.Fprintf(setup.Output, "Setting Coeus up in %s.\n", setup.Home.Root)
	if err := makeTheLayout(setup.Home); err != nil {
		return err
	}

	roots, err := setup.askWorkFolders(ctx, ask, chosen)
	if err != nil {
		return err
	}
	found := detectModels(ctx, setup)
	picked, err := setup.askModel(ctx, ask, chosen, found)
	if err != nil {
		return err
	}
	if err := setup.storeKey(ctx, chosen, picked); err != nil {
		return err
	}
	signalWanted, err := setup.askSignal(ctx, ask, chosen)
	if err != nil {
		return err
	}

	if err := writeConfiguration(setup.Home, picked, found, roots); err != nil {
		return err
	}
	return setup.finish(ctx, signalWanted)
}

// finish prints what the doctor found and the commands a new user needs.
func (setup Setup) finish(ctx context.Context, signalWanted bool) error {
	report := config.Doctor(ctx, setup.Home)
	fmt.Fprintf(setup.Output, "\n%s", report)
	fmt.Fprint(setup.Output, nextSteps(signalWanted))
	return nil
}

// nextSteps is the closing text: the five commands a new user needs, with the
// Signal one left out when Signal is switched off or signal-cli is not there.
func nextSteps(signalWanted bool) string {
	written := &strings.Builder{}
	written.WriteString("\nCoeus is ready. Here is what to type next:\n\n")
	written.WriteString("  coeus              talk to Coeus in this terminal\n")
	if signalWanted && onThePath(signalProgram) {
		written.WriteString("  coeus signal link  link Coeus to your Signal account\n")
	}
	written.WriteString("  coeus doctor       check that everything Coeus needs is here\n")
	written.WriteString("\nAnd once you are talking to it:\n\n")
	written.WriteString("  /help              the list of commands\n")
	written.WriteString("  /tasks             what Coeus is working on\n")
	return written.String()
}
