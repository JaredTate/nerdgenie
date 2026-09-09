package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// runSocketVariable names the socket to talk to, which is only ever set by a
// test. Everything else finds the socket in the home folder.
const runSocketVariable = "NERDGENIE_SOCKET"

// The bounds "nerdgenie run" works inside.
const (
	// defaultRunTimeout is how long it waits for an answer before it gives up.
	// A task is allowed an hour, and one turn a quarter of an hour, so half an
	// hour is long enough for an answer and short enough that a script which
	// forgot the flag does not hang for a day.
	defaultRunTimeout = 30 * time.Minute
	// maxRunLineBytes is the longest line it will read off the socket, which is
	// what the socket itself will write.
	maxRunLineBytes = 1 << 20
)

// runSubcommand asks the running agent one thing from a script: it attaches to
// the socket exactly as the screen does, sends one message, writes the reply on
// the ordinary output and everything else on the error output, and leaves with
// nothing on the ordinary output but the answer.
//
// It is how Nerd Genie is used from a shell script, a cron line, or another program,
// and it never runs a model of its own: the agent it talks to does the work.
var runSubcommand = subcommand{
	name: "run",
	help: "Asks the running agent one thing and prints the reply: nerdgenie run \"what is the time?\"",
	run:  runOnePrompt,
}

// runOptions are the flags "nerdgenie run" takes.
type runOptions struct {
	// yes approves anything the agent asks about instead of refusing it.
	yes bool
	// wait keeps reading until the task finishes rather than stopping at the
	// first reply.
	wait bool
	// timeout is how long to wait for an answer.
	timeout time.Duration
}

// runOnePrompt reads the flags, joins the words into one prompt, and talks to
// the agent.
func runOnePrompt(arguments []string, output io.Writer, problems io.Writer) int {
	chosen, prompt, err := readRunFlags(arguments, problems)
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie run: %v\n", err)
		return contract.ExitUsage
	}

	path, err := whereTheAgentIsListening()
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie run: %v\n", err)
		return contract.ExitBadConfiguration
	}
	connection, err := net.Dial("unix", path)
	if err != nil {
		fmt.Fprintf(problems, "nerdgenie run: nothing is listening at %s, so start the agent with \"nerdgenie serve\" and try again: %v\n", path, err)
		return contract.ExitFailure
	}
	defer func() { _ = connection.Close() }()

	if err := oneExchange(connection, chosen, prompt, output, problems); err != nil {
		fmt.Fprintf(problems, "nerdgenie run: %v\n", err)
		return contract.ExitFailure
	}
	return contract.ExitOK
}

// readRunFlags reads the command line into the options and the prompt.
func readRunFlags(arguments []string, problems io.Writer) (runOptions, string, error) {
	chosen := runOptions{}
	set := flag.NewFlagSet("nerdgenie run", flag.ContinueOnError)
	set.SetOutput(problems)
	set.BoolVar(&chosen.yes, "yes", false, "approve anything the agent asks about instead of refusing it")
	set.BoolVar(&chosen.wait, "wait", false, "wait for the whole task to finish rather than for the first reply")
	set.DurationVar(&chosen.timeout, "timeout", defaultRunTimeout, "how long to wait for an answer")

	if err := set.Parse(arguments); err != nil {
		return chosen, "", fmt.Errorf("the flags could not be read, so nothing was sent: %w", err)
	}
	prompt := strings.TrimSpace(strings.Join(set.Args(), " "))
	if prompt == "" {
		return chosen, "", errors.New("there is nothing to ask, so write the prompt in quotation marks, as in nerdgenie run \"what is the time?\"")
	}
	if chosen.timeout <= 0 {
		return chosen, "", fmt.Errorf("the timeout is %s, so give it a length of time above zero", chosen.timeout)
	}
	return chosen, prompt, nil
}

// whereTheAgentIsListening is the socket to talk to: the one the environment
// names for a test, and the home folder's own otherwise.
func whereTheAgentIsListening() (string, error) {
	if named := strings.TrimSpace(os.Getenv(runSocketVariable)); named != "" {
		return named, nil
	}
	home, err := config.HomeFolder()
	if err != nil {
		return "", err
	}
	return home.SocketFile(), nil
}

// oneExchange attaches, sends the prompt, and reads until the answer is in.
func oneExchange(connection net.Conn, chosen runOptions, prompt string, output io.Writer, problems io.Writer) error {
	if err := connection.SetDeadline(time.Now().Add(chosen.timeout)); err != nil {
		return fmt.Errorf("cannot put a deadline on the link to the agent: %w", err)
	}
	for _, sending := range []contract.SocketEnvelope{
		{Type: contract.SocketAttach},
		{Type: contract.SocketMessage, Text: prompt},
	} {
		if err := contract.EncodeSocketEnvelope(connection, sending); err != nil {
			return fmt.Errorf("cannot send the prompt to the agent: %w", err)
		}
	}
	return readUntilTheAnswerIsIn(connection, chosen, output, problems)
}

// readUntilTheAnswerIsIn reads one envelope at a time and does what each one
// asks: the reply goes on the ordinary output, everything else goes on the error
// output, and a question is answered the way the flags said.
func readUntilTheAnswerIsIn(connection net.Conn, chosen runOptions, output io.Writer, problems io.Writer) error {
	lines := bufio.NewScanner(connection)
	lines.Buffer(make([]byte, 4096), maxRunLineBytes)
	replied := false

	for lines.Scan() {
		envelope, err := contract.DecodeSocketEnvelope(lines.Bytes())
		if err != nil {
			fmt.Fprintf(problems, "the agent sent something this version cannot read: %v\n", err)
			continue
		}
		done, err := oneEnvelope(connection, envelope, chosen, output, problems, &replied)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return whyTheReadingStopped(lines.Err(), chosen, replied)
}

// oneEnvelope acts on one message from the agent and says whether the exchange
// is over.
func oneEnvelope(connection net.Conn, envelope contract.SocketEnvelope, chosen runOptions,
	output io.Writer, problems io.Writer, replied *bool) (bool, error) {
	switch envelope.Type {
	case contract.SocketReply:
		fmt.Fprintln(output, envelope.Text)
		*replied = true
		if !chosen.wait {
			return true, nil
		}
		// With --wait a workorder's reply is one of the job's task reports or
		// its own closing line; only the closing line ends the exchange, so the
		// whole job is waited on, not its first task.
		return theJobHasEnded(envelope.Text), nil
	case contract.SocketError:
		return false, errors.New(troubleIn(envelope))
	case contract.SocketPreview, contract.SocketAsk:
		return false, answerTheQuestion(connection, envelope, chosen, problems)
	case contract.SocketStatus:
		writeTheLinesWorthSeeing(envelope.Fields, problems)
		return *replied && chosen.wait && theTaskHasEnded(envelope.Fields), nil
	case contract.SocketHandoff:
		fmt.Fprintf(problems, "the agent is asking you to take over: %s\n", envelope.Text)
		return false, nil
	default:
		return false, nil
	}
}

// answerTheQuestion answers a preview or a question the way the flags said:
// approved when --yes was given, and refused otherwise, because nothing on the
// ask-me-first list happens without a person saying yes.
func answerTheQuestion(connection net.Conn, envelope contract.SocketEnvelope, chosen runOptions, problems io.Writer) error {
	fmt.Fprintf(problems, "the agent asked: %s\n", strings.TrimSpace(envelope.Title+"\n"+envelope.Text))
	answer := contract.SocketEnvelope{Type: contract.SocketDeny, ID: envelope.ID,
		Reason: "nerdgenie run was not given --yes, so nothing that needs a person's word may run"}
	if chosen.yes {
		answer = contract.SocketEnvelope{Type: contract.SocketApprove, ID: envelope.ID}
	}
	fmt.Fprintf(problems, "nerdgenie run answered %s, because --yes was %s\n", answer.Type, wasItGiven(chosen.yes))
	if err := contract.EncodeSocketEnvelope(connection, answer); err != nil {
		return fmt.Errorf("cannot answer what the agent asked: %w", err)
	}
	return nil
}

// wasItGiven says whether the flag was there, for the line that says what was
// answered and why.
func wasItGiven(yes bool) string {
	if yes {
		return "given"
	}
	return "not given"
}

// writeTheLinesWorthSeeing prints the two lines a person watching a script wants:
// what the agent is doing to a record, and which tool is running.
func writeTheLinesWorthSeeing(fields map[string]string, problems io.Writer) {
	if line := fields[contract.StatusFieldRecordLine]; line != "" {
		fmt.Fprintln(problems, line)
	}
	if line := fields[contract.StatusFieldToolLine]; line != "" {
		fmt.Fprintln(problems, line)
	}
}

// aJobsTaskLine matches a record line for one task of a job, such as
// "job 4 task 1 done", as against a plain task's "task 7 done".
var aJobsTaskLine = regexp.MustCompile(`^job [0-9]+ task [0-9]+ `)

// theJobsEnd matches the reply a job sends when it is over: finished, run every
// task with its done list unproved, or put down paused or waiting for a person.
// The lines a job sends while it keeps going — "Job N keeps working task M",
// "Job N picks task M up", "Job N, report r...", "Made job N" — are not among
// them, so --wait reads those as progress and waits on. The match is line by
// line, because a put-down carries the task's report first and the closing line
// under it.
var theJobsEnd = regexp.MustCompile(`(?m)^Job [0-9]+ (is finished|has run every task|is paused on task|is waiting on task)\b`)

// theJobHasEnded says whether a reply is a job's own closing line, which is
// what --wait waits for when a workorder became a job. A job's task lines each
// say "done", so waiting on those would stop --wait after the job's first task;
// the job itself ends only with one of the closing lines theJobsEnd reads.
func theJobHasEnded(text string) bool {
	return theJobsEnd.MatchString(text)
}

// theTaskHasEnded says whether the record line says the task is over, which is
// what --wait waits for. A job's own per-task ending is not the whole task
// ending: the job has more tasks, or it closes with a reply theJobHasEnded
// reads, so --wait must not stop on a job task's "done" line.
func theTaskHasEnded(fields map[string]string) bool {
	line := fields[contract.StatusFieldRecordLine]
	if aJobsTaskLine.MatchString(line) {
		return false
	}
	for _, ending := range []contract.RecordStatus{
		contract.StatusDone, contract.StatusFailed, contract.StatusStopped, contract.StatusWaiting,
	} {
		if strings.Contains(line, " "+string(ending)+" ") {
			return true
		}
	}
	return false
}

// troubleIn is what an error message from the agent says.
func troubleIn(envelope contract.SocketEnvelope) string {
	said := strings.TrimSpace(envelope.Text + " " + envelope.Reason)
	if said == "" {
		return "the agent reported a problem and did not say what it was"
	}
	return said
}

// whyTheReadingStopped turns the end of the stream into the plain sentence a
// person reading a script's output needs.
func whyTheReadingStopped(err error, chosen runOptions, replied bool) error {
	if replied {
		return nil
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || err == nil {
		return fmt.Errorf("the agent said nothing within %s, so it may be busy with something else; ask again or give a longer --timeout", chosen.timeout)
	}
	return fmt.Errorf("the link to the agent ended before it answered: %w", err)
}
