package browser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// stopGracePeriod is how long a worker is given to close Chrome and go quietly
// after its standard input is closed, before it is made to go.
const stopGracePeriod = 10 * time.Second

// mostLogLineBytes caps one line of the worker's own logging, so that a worker
// writing nonsense to standard error cannot fill this program's memory.
const mostLogLineBytes = 4096

// The environment the worker is handed. Everything else is left behind, because
// the worker drives another company's browser and has no business holding a key.
// The display variables are here because rule two of the protocol says the
// window is visible on the machine's own display, and Chrome cannot draw one
// without them. The design is Hermes' sanitized child environment at
// ~/Code/hermes-agent/tools/computer_use/permissions.py.
var environmentPassedOn = []string{
	"PATH",
	"HOME",
	"LANG",
	"DISPLAY",
	"WAYLAND_DISPLAY",
	"XDG_SESSION_TYPE",
	"XDG_RUNTIME_DIR",
	"XAUTHORITY",
	"DBUS_SESSION_BUS_ADDRESS",
}

// ProcessStart returns a Start that runs the browser worker as a child process,
// such as `node bin/workers/browser/main.js`. The profile folder is the agent's
// own Chrome profile and never the user's daily one; it is made with mode 0700
// before the first launch, and it lives outside the sandbox. The worker is ended
// only by the exact process identifier it was started with, never by anything
// that matches a name.
func ProcessStart(command []string, profile string, pacing Pacing, note func(format string, arguments ...any)) (Start, error) {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return nil, errors.New("the browser worker command is empty, so say which program to run and with what arguments")
	}
	if strings.TrimSpace(profile) == "" {
		return nil, errors.New("the browser worker needs its own profile folder, and it must never be the user's daily Chrome profile")
	}
	if pacing != PacingHuman && pacing != PacingFast {
		return nil, fmt.Errorf("the browser pacing %q is not one of human or fast, so ask for one of those two", pacing)
	}
	if note == nil {
		note = func(string, ...any) {}
	}

	whole := append([]string{}, command...)
	whole = append(whole, "--profile", profile, "--pacing", string(pacing))
	return func(ctx context.Context) (*Connection, error) {
		if err := makeProfileFolder(profile); err != nil {
			return nil, err
		}
		return startProcess(ctx, whole, note)
	}, nil
}

// makeProfileFolder makes the agent's own Chrome profile folder, readable by
// nobody else, because it holds the cookies that are the agent's logins.
func makeProfileFolder(profile string) error {
	if err := os.MkdirAll(profile, contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the browser profile folder %s could not be made: %w", profile, err)
	}
	if err := os.Chmod(profile, contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the browser profile folder %s could not be made private: %w", profile, err)
	}
	return checkProfileIsPrivate(profile)
}

// checkProfileIsPrivate refuses a profile folder anybody else on the machine
// could read, because its cookies are the agent's logins.
func checkProfileIsPrivate(profile string) error {
	about, err := os.Stat(profile)
	if err != nil {
		return fmt.Errorf("the browser profile folder %s could not be looked at: %w", profile, err)
	}
	if about.Mode().Perm()&^fs.FileMode(0o700) != 0 {
		return fmt.Errorf("the browser profile folder %s is readable by somebody other than this account, so run chmod 700 %s before using the browser", profile, profile)
	}
	return nil
}

// startProcess starts one worker and wires its three streams.
func startProcess(ctx context.Context, command []string, note func(format string, arguments ...any)) (*Connection, error) {
	running := exec.CommandContext(ctx, command[0], command[1:]...)
	running.Env = workerEnvironment()
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	requests, err := running.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("the browser worker's input could not be opened: %w", err)
	}
	answers, err := running.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("the browser worker's output could not be read: %w", err)
	}
	complaints, err := running.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("the browser worker's log could not be read: %w", err)
	}
	if err := running.Start(); err != nil {
		return nil, fmt.Errorf("the browser worker %s could not be started: %w", command[0], err)
	}

	go readLog(complaints, note)
	pid := running.Process.Pid
	return &Connection{
		Requests:  requests,
		Responses: answers,
		ProcessID: pid,
		Stop:      func() error { return stopProcess(running, requests, pid) },
	}, nil
}

// stopProcess ends one worker: its input is closed, which is how the worker is
// asked to close Chrome and go, and then the process group started under that
// one exact process identifier is asked to go, and made to go if it will not.
// No process is ever found by name.
func stopProcess(running *exec.Cmd, requests io.Closer, pid int) error {
	_ = requests.Close()
	done := make(chan error, 1)
	go func() { done <- running.Wait() }()
	select {
	case <-done:
		return nil
	case <-time.After(stopGracePeriod):
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("the browser worker with process id %d would not take a stop signal: %w", pid, err)
	}
	select {
	case <-done:
		return nil
	case <-time.After(stopGracePeriod):
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("the browser worker with process id %d could not be killed: %w", pid, err)
	}
	<-done
	return nil
}

// readLog copies the worker's own log lines into this program's log, one line at
// a time and each one capped.
func readLog(complaints io.Reader, note func(format string, arguments ...any)) {
	lines := bufio.NewScanner(complaints)
	lines.Buffer(make([]byte, 0, 1024), mostLogLineBytes)
	for lines.Scan() {
		note("browser worker: %s", lines.Text())
	}
}

// workerEnvironment is the environment the worker is handed, which carries the
// display Chrome draws on and nothing secret.
func workerEnvironment() []string {
	handed := []string{}
	for _, name := range environmentPassedOn {
		if value, set := os.LookupEnv(name); set {
			handed = append(handed, name+"="+value)
		}
	}
	return handed
}
