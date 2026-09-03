package desktop

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// killGracePeriod is how long a worker is given to go quietly before it is made
// to go.
const killGracePeriod = 2 * time.Second

// maximumLogLineLength caps one line of the worker's own logging, so that a
// worker writing nonsense to standard error cannot fill this program's memory.
const maximumLogLineLength = 4096

// The environment the worker is handed. Everything else is left behind: the
// driver is another project's program and has no business holding a key, which
// is rule seven in worker/desktop/PROTOCOL.md. The design is Hermes' sanitized
// child environment at ~/Code/hermes-agent/tools/computer_use/permissions.py.
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

// ProcessStart returns a Start that runs the desktop worker as a child process,
// such as `node bin/workers/desktop/main.js`. The worker is killed only by its
// exact process identifier, never by anything that matches a name.
func ProcessStart(command []string, note func(format string, arguments ...any)) (Start, error) {
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return nil, errors.New("the desktop worker command is empty, so say which program to run and with what arguments")
	}
	if note == nil {
		note = func(string, ...any) {}
	}
	whole := append([]string{}, command...)

	return func(ctx context.Context) (*Connection, error) {
		return startProcess(ctx, whole, note)
	}, nil
}

// startProcess starts one worker and wires its three streams.
func startProcess(ctx context.Context, command []string, note func(format string, arguments ...any)) (*Connection, error) {
	running := exec.CommandContext(ctx, command[0], command[1:]...)
	running.Env = workerEnvironment()
	running.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	requests, err := running.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("the desktop worker's input could not be opened: %w", err)
	}
	responses, err := running.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("the desktop worker's output could not be read: %w", err)
	}
	complaints, err := running.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("the desktop worker's log could not be read: %w", err)
	}
	if err := running.Start(); err != nil {
		return nil, fmt.Errorf("the desktop worker %s could not be started: %w", command[0], err)
	}

	go readLog(complaints, note)
	pid := running.Process.Pid
	return &Connection{
		Requests:  requests,
		Responses: responses,
		ProcessID: pid,
		Stop:      func() error { return stopProcess(running, requests, pid) },
	}, nil
}

// stopProcess ends one worker: its input is closed so it can stop on its own,
// then its exact process group is asked to go, and made to go if it will not.
func stopProcess(running *exec.Cmd, requests io.Closer, pid int) error {
	_ = requests.Close()
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("the desktop worker with process id %d would not take a stop signal: %w", pid, err)
	}
	done := make(chan error, 1)
	go func() { done <- running.Wait() }()
	select {
	case <-done:
		return nil
	case <-time.After(killGracePeriod):
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("the desktop worker with process id %d could not be killed: %w", pid, err)
	}
	<-done
	return nil
}

// readLog copies the worker's own log lines into this program's log, one line
// at a time and each one capped.
func readLog(complaints io.Reader, note func(format string, arguments ...any)) {
	lines := bufio.NewScanner(complaints)
	lines.Buffer(make([]byte, 0, 1024), maximumLogLineLength)
	for lines.Scan() {
		note("desktop worker: %s", lines.Text())
	}
}

// workerEnvironment is the environment the worker is handed, which carries the
// display it draws on and nothing secret.
func workerEnvironment() []string {
	handed := []string{}
	for _, name := range environmentPassedOn {
		if value, set := os.LookupEnv(name); set {
			handed = append(handed, name+"="+value)
		}
	}
	return handed
}
