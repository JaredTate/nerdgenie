package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/JaredTate/nerdgenie/internal/clock"
	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/permission"
	"github.com/JaredTate/nerdgenie/internal/replay"
)

// replaySubcommand runs one recorded task again against the code as it stands
// now and says whether it ends where it ended when it was recorded. It is the
// fourth tier of the self-fixing table in design section 11: the user makes a
// fix and then replays the task that went wrong.
//
// Nothing here touches the world. The model and every tool answer out of the
// event log, the replayed run is written into a throwaway database that is
// thrown away when the command ends, and no command a done line names is run,
// because the replay is given no sandbox.
//
// The orchestrator adds this value to the table in main.go.
var replaySubcommand = subcommand{
	name: "replay",
	help: "Runs a recorded task again against the code as it stands now, and says where the two runs differ.",
	run:  runTheReplaySubcommand,
}

// runTheReplaySubcommand reads the flags, replays the task, and prints what the
// replay found.
func runTheReplaySubcommand(arguments []string, output io.Writer, problems io.Writer) int {
	set := flag.NewFlagSet("nerdgenie replay", flag.ContinueOnError)
	set.SetOutput(problems)
	asTest := set.Bool("as-test", false, "also write a Go test under test/replays that replays this task")
	into := set.String("into", ".", "the folder the generated test is written under, which is the top of the repository")
	if err := set.Parse(arguments); err != nil {
		fmt.Fprintf(problems, "nerdgenie replay: the flags could not be read, so nothing was replayed: %v\n", err)
		return contract.ExitUsage
	}
	if len(set.Args()) != 1 {
		fmt.Fprintln(problems, "nerdgenie replay: name exactly one task to replay, as in \"nerdgenie replay 17\"")
		return contract.ExitUsage
	}
	return replayOneTask(set.Args()[0], *asTest, *into, output, problems)
}

// replayOneTask opens everything the replay needs, runs it, and prints the
// report. The exit code says what the replay found, so that a script can use
// this command as the test it is.
func replayOneTask(taskID string, asTest bool, into string, output io.Writer, problems io.Writer) int {
	ctx := context.Background()
	parts, code := openWhatAReplayNeeds(problems)
	if code != contract.ExitOK {
		return code
	}
	defer parts.close(problems)

	recording, err := replay.Read(ctx, parts.recorded, taskID)
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie replay: %v\n", err)
		return contract.ExitFailure
	}
	result, err := replay.RunRecording(ctx, parts.options, recording)
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie replay: %v\n", err)
		return contract.ExitFailure
	}
	fmt.Fprintln(output, result.Report)

	if asTest {
		if code := writeTheReplayTest(recording, into, output, problems); code != contract.ExitOK {
			return code
		}
	}
	if !result.Passed {
		return contract.ExitFailure
	}
	return contract.ExitOK
}

// writeTheReplayTest writes the Go test that replays this recording and says
// where the two files went.
func writeTheReplayTest(recording replay.Recording, into string, output io.Writer, problems io.Writer) int {
	generated, err := replay.AsTest(recording)
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie replay: %v\n", err)
		return contract.ExitFailure
	}
	if err := generated.WriteInto(into); err != nil {
		fmt.Fprintf(problems, "nerdgenie replay: %v\n", err)
		return contract.ExitFailure
	}
	fmt.Fprintf(output, "wrote %s and %s. Run it with \"go test ./%s/...\".\n",
		filepath.Join(into, filepath.FromSlash(generated.TestPath)),
		filepath.Join(into, filepath.FromSlash(generated.FixturePath)),
		replay.TestsFolder)
	return contract.ExitOK
}

// replayParts is everything one run of the replay subcommand opens, and the
// two things it has to close again.
type replayParts struct {
	// recorded is the agent's own event log, which is read and never written.
	recorded *log.Log
	// throwaway is the log the replayed run is written into.
	throwaway *log.Log
	// folder holds the throwaway log and is removed when the command ends.
	folder string
	// options is what the replay is run with.
	options replay.Options
}

// close puts back everything the replay opened, including the folder the
// throwaway log sat in, because a replay leaves nothing behind.
func (parts replayParts) close(problems io.Writer) {
	for _, opened := range []*log.Log{parts.throwaway, parts.recorded} {
		if opened == nil {
			continue
		}
		if err := opened.Close(); err != nil {
			fmt.Fprintf(problems, "nerdgenie replay: cannot close an event log: %v\n", err)
		}
	}
	if parts.folder == "" {
		return
	}
	if err := os.RemoveAll(parts.folder); err != nil {
		fmt.Fprintf(problems, "nerdgenie replay: cannot remove the throwaway log in %s: %v\n", parts.folder, err)
	}
}

// openWhatAReplayNeeds opens the home folder, the configuration, the two event
// logs, the working context, and the permission function as it stands now.
func openWhatAReplayNeeds(problems io.Writer) (replayParts, int) {
	ctx := context.Background()
	home, settings, code := homeAndSettings(problems, "replay")
	if code != contract.ExitOK {
		return replayParts{}, code
	}
	parts := replayParts{}
	recorded, err := log.Open(ctx, home.DatabaseFile())
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie replay: %v\n", err)
		return replayParts{}, contract.ExitFailure
	}
	parts.recorded = recorded

	folder, err := os.MkdirTemp("", "nerdgenie-replay-")
	if err != nil {
		parts.close(problems)
		fmt.Fprintf(problems, "nerdgenie replay: cannot make a folder for the throwaway log: %v\n", err)
		return replayParts{}, contract.ExitFailure
	}
	parts.folder = folder
	throwaway, err := log.Open(ctx, filepath.Join(folder, "replay.db"))
	if err != nil {
		parts.close(problems)
		fmt.Fprintf(problems, "nerdgenie replay: %v\n", err)
		return replayParts{}, contract.ExitFailure
	}
	parts.throwaway = throwaway

	options, err := theReplayOptions(home, settings, recorded, throwaway)
	if err != nil {
		parts.close(problems)
		fmt.Fprintf(problems, "nerdgenie replay: %v\n", err)
		return replayParts{}, contract.ExitFailure
	}
	parts.options = options
	return parts, contract.ExitOK
}

// theReplayOptions builds the working context and the permission function the
// running program builds, so that a replay tests the prompt and the rulebook
// this machine has now rather than copies of them.
func theReplayOptions(home contract.Home, settings contract.Config,
	recorded *log.Log, throwaway *log.Log) (replay.Options, error) {
	built, err := workingcontext.New(workingcontext.Options{
		Home:            home,
		MemoryCaps:      settings.MemoryCaps,
		MaxOutputTokens: settings.Caps.OutputTokensPerCall,
	})
	if err != nil {
		return replay.Options{}, err
	}
	rulings, err := permission.New(settings, clock.System())
	if err != nil {
		return replay.Options{}, err
	}
	return replay.Options{
		From:       recorded,
		Into:       throwaway,
		Context:    loop.TheWorkingContext(built),
		Permission: rulings,
		Clock:      clock.System(),
		Caps:       settings.Caps,
	}, nil
}
