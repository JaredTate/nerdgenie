package shell

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/orientation"
)

// ServeWatchFor is how long a serve watches for its port before saying nothing
// listens yet. Ten seconds is the yield a run gets, and every server the logs
// hold was listening within it.
const ServeWatchFor = 10 * time.Second

// ServeCheckEvery is how often a serve looks at the socket tables while it
// watches, so that a server is answered within half a second of listening.
const ServeCheckEvery = 500 * time.Millisecond

// theSocketTables are the kernel's tables of TCP sockets, which the orientation
// block reads for the same purpose.
var theSocketTables = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// checkServe holds the rules a serve must satisfy: the rules of a run, and a
// port that is a port when one is named.
func checkServe(asked Call) error {
	if err := checkRun(asked); err != nil {
		return err
	}
	if asked.Port != 0 {
		return checkPort(asked.Port)
	}
	return nil
}

// checkPort refuses a port no machine has.
func checkPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("the port %d is not one a machine has, so give a port between 1 and 65535", port)
	}
	return nil
}

// serve starts a command that is meant to keep running and answers the moment
// it listens on a port, instead of after ten seconds with an id to poll. The
// play-test task of the fresh game build started a server and then asked
// whether it was done for the rest of the task, 131 poll-only rounds across
// one day's logs, because a server never finishes.
func (tool *Tool) serve(ctx context.Context, asked Call) (contract.ToolOutput, error) {
	if tool.settings.Sandbox == nil {
		return contract.ToolOutput{}, fmt.Errorf("this tool has no sandbox to run commands in, so wire the sandbox in before using it")
	}
	if err := tool.settings.Sandbox.Available(); err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the shell tool is off because the sandbox cannot run, and nothing runs outside the fence: %w", err)
	}
	before, _ := orientation.EveryListeningPort(tool.socketTables()...)
	entry, err := tool.running.add(asked.Command, tool.settings.Timeout, tool.now(), tool.workOf(asked))
	if err != nil {
		return contract.ToolOutput{}, err
	}
	for waited := time.Duration(0); waited < ServeWatchFor; waited += ServeCheckEvery {
		finished, err := tool.waitFor(ctx, entry, ServeCheckEvery)
		if err != nil {
			return contract.ToolOutput{}, err
		}
		if finished {
			return contract.ToolOutput{Text: entry.finishedText()}, nil
		}
		if listening := tool.newlyListening(before, asked.Port); len(listening) > 0 {
			return contract.ToolOutput{Text: fmt.Sprintf("listening on %s as %s; check it with action check, stop it with kill\n", portWords(listening), entry.id)}, nil
		}
	}
	return contract.ToolOutput{Text: stillStartingText(entry.id, asked.Port)}, nil
}

// socketTables are the tables a serve watches: the settings' when a test
// named some, and the kernel's otherwise.
func (tool *Tool) socketTables() []string {
	if len(tool.settings.SocketTables) > 0 {
		return tool.settings.SocketTables
	}
	return theSocketTables
}

// newlyListening is the ports listening now that were not before, or the one
// port asked for when it listens now, whether or not it did before.
func (tool *Tool) newlyListening(before []int, asked int) []int {
	now, _ := orientation.EveryListeningPort(tool.socketTables()...)
	if asked != 0 {
		if slices.Contains(now, asked) {
			return []int{asked}
		}
		return nil
	}
	listening := []int{}
	for _, port := range now {
		if !slices.Contains(before, port) {
			listening = append(listening, port)
		}
	}
	return listening
}

// stillStartingText is what a serve says when nothing listened within the
// watch: the id, so that the model can tail or kill it, and the port to check
// later when one was named.
func stillStartingText(id string, port int) string {
	where := "nothing listens yet; check the port later, or tail or kill it"
	if port != 0 {
		where = fmt.Sprintf("nothing listens on %d yet; check it later, or tail or kill it", port)
	}
	return fmt.Sprintf("still starting after %s as %s; %s\n", ServeWatchFor, id, where)
}

// portWords is a list of ports as words.
func portWords(ports []int) string {
	words := make([]string, 0, len(ports))
	for _, port := range ports {
		words = append(words, strconv.Itoa(port))
	}
	return strings.Join(words, " ")
}
