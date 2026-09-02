package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// DescribeTimeout is how long one of the user's own tools has to say what it is.
// Startup waits for every tool in the folder in turn, so a tool that hangs must
// not be able to hold the agent up for longer than a glance.
const DescribeTimeout = 10 * time.Second

// MaxDescribeBytes is how much of a tool's description is read before the read
// is given up on, because a program asked what it is can print anything at all.
const MaxDescribeBytes = 64 << 10

// AddUserTools registers every executable in the tools folder under the agent's
// home, asking each one what it is with the describe flag and reading a tool
// specification back as JSON. A tool that cannot be asked, or whose answer will
// not parse, is skipped with one logged line naming the file and the problem,
// because one bad script in a folder must not stop the agent from starting.
func (registry *Registry) AddUserTools(ctx context.Context) error {
	folder := registry.settings.Home.ToolsFolder()
	entries, err := os.ReadDir(folder)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the tools folder %s, so check that it is a folder the agent may read: %w", folder, err)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(folder, entry.Name())
		if err := registry.addOneUserTool(ctx, path); err != nil {
			registry.note(fmt.Sprintf("coeus skipped the tool %s: %v", path, err))
		}
	}
	return nil
}

// addOneUserTool asks one program what it is and registers it when the answer is
// something the model could use.
func (registry *Registry) addOneUserTool(ctx context.Context, path string) error {
	described, err := runUserProgram(ctx, path, DescribeTimeout, nil, contract.UserToolDescribeFlag)
	if err != nil {
		return err
	}
	spec, err := contract.DecodeUserToolSpec(described)
	if err != nil {
		return err
	}
	return registry.Add(&userTool{path: path, spec: spec, timeout: registry.settings.toolTimeout()})
}

// note says one line about something the registry skipped, through the function
// the settings named, or to the standard library's logger when they named none.
func (registry *Registry) note(line string) {
	if registry.settings.Note != nil {
		registry.settings.Note(line)
		return
	}
	log.Print(line)
}

// userTool is one of the user's own programs, run with the model's input on its
// standard input and answering with plain text on its standard output.
type userTool struct {
	path    string
	spec    contract.ToolSpec
	timeout time.Duration
}

// Spec is what the program said it was when it was asked.
func (tool *userTool) Spec() contract.ToolSpec { return tool.spec }

// Run hands the program the model's arguments as JSON on its standard input and
// returns what it wrote on its standard output.
func (tool *userTool) Run(ctx context.Context, input json.RawMessage) (contract.ToolOutput, error) {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	written, err := runUserProgram(ctx, tool.path, tool.timeout, input)
	if err != nil {
		return contract.ToolOutput{}, fmt.Errorf("the tool %s did not finish: %w", tool.spec.Name, err)
	}
	return contract.ToolOutput{Text: string(written)}, nil
}

// runUserProgram runs one of the user's programs in its own process group under
// a timeout, feeding it what the caller passed on its standard input and
// returning what it wrote on its standard output. A program that quits badly is
// reported with what it said on its error output, because that is the part a
// person can act on.
func runUserProgram(ctx context.Context, path string, timeout time.Duration, standardInput []byte, arguments ...string) ([]byte, error) {
	running, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	program := exec.CommandContext(running, path, arguments...)
	program.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	program.Cancel = func() error { return killGroup(program) }
	program.WaitDelay = time.Second
	program.Stdin = bytes.NewReader(standardInput)
	written := &bytes.Buffer{}
	complaint := &bytes.Buffer{}
	program.Stdout = &cappedWriter{into: written, room: MaxDescribeBytes}
	program.Stderr = &cappedWriter{into: complaint, room: MaxDescribeBytes}

	if err := program.Run(); err != nil {
		return nil, fmt.Errorf("%w, and it said %q on its error output", err, strings.TrimSpace(complaint.String()))
	}
	return written.Bytes(), nil
}

// killGroup stops a program and everything it started, because a tool that
// starts children and then outruns its timeout must not leave them behind.
func killGroup(program *exec.Cmd) error {
	if program.Process == nil {
		return nil
	}
	if err := syscall.Kill(-program.Process.Pid, syscall.SIGKILL); err != nil {
		return program.Process.Kill()
	}
	return nil
}

// cappedWriter keeps what is written to it up to a fixed number of bytes and
// quietly drops the rest, so that a program printing forever cannot fill memory.
type cappedWriter struct {
	into *bytes.Buffer
	room int
}

// Write keeps as much as there is room for and reports the whole write as taken,
// because a program stopped by a short write would report the wrong problem.
func (writer *cappedWriter) Write(written []byte) (int, error) {
	if writer.room > 0 {
		keeping := written
		if len(keeping) > writer.room {
			keeping = keeping[:writer.room]
		}
		writer.into.Write(keeping)
		writer.room -= len(keeping)
	}
	return len(written), nil
}
