//go:build integration

package functional

// Starting the real browser worker from worker/browser and speaking to it the
// way the Go side will: JSON-RPC 2.0 over its standard input and output, one
// object per line, one request at a time. The protocol is
// worker/browser/PROTOCOL.md and the shapes are the ones in internal/contract,
// which is why every answer here is decoded into a contract type: a worker whose
// JSON no longer fits the shared types fails the test on the spot.
//
// This file needs the development machine. It launches the real Chrome on the
// machine's own display, so a missing Chrome, a missing Node, or a worker that
// will not build is a loud failure rather than a skip.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// browserRequestLimit bounds one request. Every method the worker has is bounded
// far more tightly by the worker itself, so this only ever fires when the worker
// has stopped answering at all.
const browserRequestLimit = 90 * time.Second

// The bounds on the two commands that make the worker runnable. Fetching
// packages reaches the network, so it is given the longer one.
const (
	installLimit = 5 * time.Minute
	buildLimit   = 3 * time.Minute
)

// browserWorkerStopLimit is how long the worker is given to close Chrome and go
// after its standard input is closed, before it is ended by the pid it was
// started with. No process is ever ended by name.
const browserWorkerStopLimit = 30 * time.Second

// browserWorker is one running worker: the child process, the two pipes, and the
// running number that labels each request.
type browserWorker struct {
	command  *exec.Cmd
	toWorker io.WriteCloser
	answers  <-chan string
	logPath  string
	nextID   int
}

// jsonRPCAnswer is one line the worker wrote back.
type jsonRPCAnswer struct {
	Result json.RawMessage `json:"result"`
	Error  *jsonRPCError   `json:"error"`
}

// jsonRPCError is the error half of an answer, as the error table in PROTOCOL.md
// describes it.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// startBrowserWorker builds the worker if it needs building, starts it with a
// throwaway profile folder, and stops it at the end of the test.
func startBrowserWorker(t *testing.T) *browserWorker {
	t.Helper()
	entry := builtWorkerEntry(t)
	node := programNamed(t, "node")
	profile := throwawayFolder(t, "coeus-browser-profile-")

	// The worker's pacing is human by default, which is right in front of a real
	// site and far too slow for a test that types a password one key at a time.
	command := exec.Command(node, entry, "--profile", profile, "--pacing", "fast")
	logFile := workerLogFile(t)
	command.Stderr = logFile
	toWorker, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("cannot open the pipe the worker reads its requests from: %v", err)
	}
	fromWorker, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("cannot open the pipe the worker writes its answers to: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("cannot start the browser worker with %s: %v", node, err)
	}

	worker := &browserWorker{
		command:  command,
		toWorker: toWorker,
		answers:  readAnswerLines(fromWorker),
		logPath:  logFile.Name(),
	}
	t.Cleanup(func() { worker.stop(t) })
	return worker
}

// readAnswerLines reads one line at a time off the worker's standard output and
// hands each one on. The buffer is a megabyte because a screenshot comes back as
// one line of base64.
func readAnswerLines(fromWorker io.Reader) <-chan string {
	lines := make(chan string, 8)
	go func() {
		defer close(lines)
		reader := bufio.NewReaderSize(fromWorker, 1<<20)
		for {
			line, err := reader.ReadString('\n')
			if strings.TrimSpace(line) != "" {
				lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}

// result sends one request and gives back the result, failing the test when the
// worker answered with an error instead. The raw JSON comes back too, because
// the promise that loginFill never returns a secret is a promise about every
// byte of the answer and not only about the fields a test reads.
func (worker *browserWorker) result(t *testing.T, method string, params map[string]any) json.RawMessage {
	t.Helper()
	worker.nextID++
	request := map[string]any{"jsonrpc": "2.0", "id": worker.nextID, "method": method, "params": params}
	line, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("cannot write the %s request as JSON: %v", method, err)
	}
	if _, err := worker.toWorker.Write(append(line, '\n')); err != nil {
		t.Fatalf("cannot send the %s request to the worker: %v%s", method, err, worker.said())
	}

	var answer jsonRPCAnswer
	if err := json.Unmarshal([]byte(worker.waitForAnswer(t, method)), &answer); err != nil {
		t.Fatalf("the worker's answer to %s was not JSON: %v%s", method, err, worker.said())
	}
	if answer.Error != nil {
		t.Fatalf("the worker refused %s with %d: %s%s",
			method, answer.Error.Code, answer.Error.Message, worker.said())
	}
	return answer.Result
}

// waitForAnswer waits for the one line this request is owed, or fails the test
// with everything the worker said on its way to not answering.
func (worker *browserWorker) waitForAnswer(t *testing.T, method string) string {
	t.Helper()
	select {
	case line, open := <-worker.answers:
		if !open {
			t.Fatalf("the worker stopped before it answered %s%s", method, worker.said())
		}
		return line
	case <-time.After(browserRequestLimit):
		t.Fatalf("the worker did not answer %s within %s%s", method, browserRequestLimit, worker.said())
		return ""
	}
}

// stop closes the worker's standard input, which is how it is asked to close
// Chrome and go, and ends it by the exact pid it was started with if it will not.
func (worker *browserWorker) stop(t *testing.T) {
	t.Helper()
	_ = worker.toWorker.Close()
	gone := make(chan error, 1)
	go func() { gone <- worker.command.Wait() }()
	select {
	case <-gone:
	case <-time.After(browserWorkerStopLimit):
		t.Errorf("the worker did not stop within %s, so it was ended by its pid %d",
			browserWorkerStopLimit, worker.command.Process.Pid)
		_ = worker.command.Process.Kill()
		<-gone
	}
}

// said is everything the worker wrote to its standard error, which is where its
// own logging goes, ready to be tacked onto a failure message.
func (worker *browserWorker) said() string {
	written, err := os.ReadFile(worker.logPath)
	if err != nil || len(written) == 0 {
		return ""
	}
	return "\nthe worker said:\n" + string(written)
}

// workerLogFile is where the worker's own logging is kept for the length of the
// test, so that a failure can quote it.
func workerLogFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "browser-worker.log"))
	if err != nil {
		t.Fatalf("cannot make the file the worker's logging is kept in: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// throwawayFolder is a folder for the length of one test. Chrome writes its last
// files as it is going, so the removal is tried a few times rather than once.
func throwawayFolder(t *testing.T, prefix string) string {
	t.Helper()
	folder, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("cannot make a throwaway folder for the browser: %v", err)
	}
	t.Cleanup(func() {
		for attempt := 0; attempt < 20; attempt++ {
			if err := os.RemoveAll(folder); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Logf("the throwaway folder %s could not be taken away and is left behind", folder)
	})
	return folder
}

// programNamed finds one program on the path, and falls back to the Node folder
// the development machine keeps its Node in, because a shell that never read the
// user's profile has neither node nor npm on its path.
func programNamed(t *testing.T, name string) string {
	t.Helper()
	if found, err := exec.LookPath(name); err == nil {
		return found
	}
	fallback := filepath.Join(os.Getenv("HOME"), ".nvm", "versions", "node", "v24.18.0", "bin", name)
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	t.Fatalf("cannot find %s on the path or at %s, and the browser worker needs it", name, fallback)
	return ""
}

// The worker is built once per test binary, however many tests ask for it.
var (
	buildOnce    sync.Once
	builtEntry   string
	buildProblem error
)

// builtWorkerEntry is the path of the built worker, building it first when it is
// missing or older than its own source.
func builtWorkerEntry(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() { builtEntry, buildProblem = buildTheWorker(t) })
	if buildProblem != nil {
		t.Fatalf("the browser worker could not be built: %v", buildProblem)
	}
	return builtEntry
}

// buildTheWorker fetches the worker's packages if they are missing and compiles
// its TypeScript if the build is missing or stale, and gives back the file Node
// runs.
func buildTheWorker(t *testing.T) (string, error) {
	t.Helper()
	folder, err := filepath.Abs(filepath.Join("..", "..", "worker", "browser"))
	if err != nil {
		return "", fmt.Errorf("cannot work out where the browser worker lives: %w", err)
	}
	npm := programNamed(t, "npm")
	if _, err := os.Stat(filepath.Join(folder, "node_modules")); err != nil {
		if err := runIn(folder, npm, []string{"ci"}, installLimit); err != nil {
			return "", err
		}
	}
	entry := filepath.Join(folder, "dist", "main.js")
	if stale, err := olderThanItsSource(entry, filepath.Join(folder, "src")); err != nil {
		return "", err
	} else if stale {
		if err := runIn(folder, npm, []string{"run", "build"}, buildLimit); err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(entry); err != nil {
		return "", fmt.Errorf("the worker was built but %s is not there: %w", entry, err)
	}
	return entry, nil
}

// olderThanItsSource says whether the built file is missing or older than the
// newest file it was built from.
func olderThanItsSource(built string, source string) (bool, error) {
	about, err := os.Stat(built)
	if err != nil {
		return true, nil
	}
	newest := time.Time{}
	walked := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if walked != nil {
		return false, fmt.Errorf("cannot look at the worker source in %s: %w", source, walked)
	}
	return newest.After(about.ModTime()), nil
}

// runIn runs one command in one folder, bounded, and turns a failure into an
// error that quotes everything the command said.
func runIn(folder string, program string, arguments []string, limit time.Duration) error {
	ctx, stop := context.WithTimeout(context.Background(), limit)
	defer stop()
	command := exec.CommandContext(ctx, program, arguments...)
	command.Dir = folder
	said, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running %s %s in %s failed after at most %s: %w, and it said: %s",
			program, strings.Join(arguments, " "), folder, limit, err, said)
	}
	return nil
}
