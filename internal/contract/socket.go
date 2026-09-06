package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SocketMessageType is one kind of message on the local socket, which the
// terminal and any later screen use to attach to the running program. The socket
// carries one JSON object per line.
type SocketMessageType string

// The message types a screen sends to the program.
const (
	// SocketMessage is something the user typed for the model.
	SocketMessage SocketMessageType = "message"
	// SocketCommand is a slash command.
	SocketCommand SocketMessageType = "command"
	// SocketApprove answers a preview with yes.
	SocketApprove SocketMessageType = "approve"
	// SocketDeny answers a preview with no.
	SocketDeny SocketMessageType = "deny"
	// SocketSecret carries what the user typed at a masked prompt.
	SocketSecret SocketMessageType = "secret"
	// SocketAttach asks to start receiving the event stream.
	SocketAttach SocketMessageType = "attach"
	// SocketDetach asks to stop receiving it.
	SocketDetach SocketMessageType = "detach"
	// SocketCancel withdraws from a preview or a masked prompt by id, which is
	// what Escape sends, so the program stops waiting at once.
	SocketCancel SocketMessageType = "cancel"
	// SocketShow asks the program for the full text of one thing the screen
	// only has a line for, with no model call: a result by its id, such as
	// r27, with the task it belongs to in Fields["task"]; a task's whole
	// record with Fields["task"]; or a job's with Fields["job"]. The program
	// answers the asking screen alone with a SocketShown. Fields["id"] names
	// the result; a request naming only a task or a job asks for its record.
	SocketShow SocketMessageType = "show"
)

// The message types the program sends to a screen.
const (
	// SocketDelta is one piece of a reply as the model writes it.
	SocketDelta SocketMessageType = "delta"
	// SocketReply is a finished reply.
	SocketReply SocketMessageType = "reply"
	// SocketPreview shows exactly what is about to happen and waits.
	SocketPreview SocketMessageType = "preview"
	// SocketAsk is the model asking the user a question in plain text.
	SocketAsk SocketMessageType = "ask"
	// SocketHandoff gives the browser or the desktop to the user, with a picture.
	SocketHandoff SocketMessageType = "handoff"
	// SocketStatus is what the status command reports.
	SocketStatus SocketMessageType = "status"
	// SocketError says something went wrong, in plain words.
	SocketError SocketMessageType = "error"
	// SocketShown answers a SocketShow: Fields carry back the "id", "task" and
	// "job" it was asked with, and Text carries the full text, which is the
	// stored result or the printed record. A thing the program cannot find
	// comes back as a SocketError instead.
	SocketShown SocketMessageType = "shown"
)

// FromScreen says whether a screen sends this kind of message.
func (kind SocketMessageType) FromScreen() bool {
	switch kind {
	case SocketMessage, SocketCommand, SocketApprove, SocketDeny,
		SocketSecret, SocketAttach, SocketDetach, SocketCancel, SocketShow:
		return true
	default:
		return false
	}
}

// FromProgram says whether the program sends this kind of message.
func (kind SocketMessageType) FromProgram() bool {
	switch kind {
	case SocketDelta, SocketReply, SocketPreview, SocketAsk,
		SocketHandoff, SocketStatus, SocketError, SocketShown:
		return true
	default:
		return false
	}
}

// SocketEnvelope is one message on the local socket. Every kind uses the same
// struct and fills in the fields it needs, so a screen can read a message it
// does not understand without failing.
type SocketEnvelope struct {
	// Type says which kind of message this is.
	Type SocketMessageType `json:"type"`
	// ID identifies a preview or a question, so that "/approve 3" can answer it.
	ID string `json:"id,omitempty"`
	// TaskID says which task the message belongs to, or is empty.
	TaskID string `json:"taskId,omitempty"`
	// Text is the message body: what the user typed, a reply, a delta, the body
	// of a preview, or the text of an error.
	Text string `json:"text,omitempty"`
	// Title is the one line above a preview or a handoff.
	Title string `json:"title,omitempty"`
	// Attachments are the paths of files sent with the message, such as the
	// screenshot on a handoff.
	Attachments []string `json:"attachments,omitempty"`
	// Secret is what the user typed at a masked prompt. It is never logged.
	Secret string `json:"secret,omitempty"`
	// Reason says why something was denied or why an error happened.
	Reason string `json:"reason,omitempty"`
	// Fields carries the name-and-value pairs of a status message.
	Fields map[string]string `json:"fields,omitempty"`
	// MaskInput, on an ask, tells the screen to hide what the person types,
	// because the answer is a secret that comes back in a secret envelope.
	MaskInput bool `json:"maskInput,omitempty"`
	// Reset, on a delta, says the partial reply shown so far is withdrawn,
	// because the call behind it failed and is being tried again; the screen
	// clears what it drew and the deltas that follow start the reply over.
	Reset bool `json:"reset,omitempty"`
	// Clear, on a reply, tells the screen to empty its transcript before it
	// shows the reply, because the person asked for a clean screen with the
	// clear command. What the program knows about itself stays where it was:
	// the header, the status strip and the side panel are drawn from the status.
	Clear bool `json:"clear,omitempty"`
}

// ApproveAlwaysText is the text an approve envelope carries when the person
// chose "always for the session" rather than this once.
const ApproveAlwaysText = "always"

// The names of the fields a status envelope carries, which the program fills
// and the screen reads, so that the two agree on one spelling. A screen ignores
// a field it does not know.
const (
	// StatusFieldModel is the alias of the model in use.
	StatusFieldModel = "model"
	// StatusFieldModelFile is the model file the local daemon has loaded, by
	// its base name, or the server's model name when the daemon does not say.
	StatusFieldModelFile = "modelFile"
	// StatusFieldPromptSpeed is how many tokens a second the last call's
	// prompt was read at, whole, when the provider reports it.
	StatusFieldPromptSpeed = "promptSpeed"
	// StatusFieldOutputSpeed is how many tokens a second the last call's
	// answer was written at, whole, when the provider reports it.
	StatusFieldOutputSpeed = "outputSpeed"
	// StatusFieldTask is the running task's number, or empty.
	StatusFieldTask = "task"
	// StatusFieldTaskState is the running task's record status.
	StatusFieldTaskState = "taskState"
	// StatusFieldTokensIn is the session's input tokens so far.
	StatusFieldTokensIn = "tokensIn"
	// StatusFieldTokensOut is the session's output tokens so far.
	StatusFieldTokensOut = "tokensOut"
	// StatusFieldCost is the session's cost in dollars when known.
	StatusFieldCost = "cost"
	// StatusFieldBudget is the task's budget line in plain words.
	StatusFieldBudget = "budget"
	// StatusFieldState is one of the screen state words below.
	StatusFieldState = "state"
	// StatusFieldTool is the tool in use when the state is using a tool.
	StatusFieldTool = "tool"
	// StatusFieldToolLine is the one dim line for the tool call in progress.
	StatusFieldToolLine = "toolLine"
	// StatusFieldCommands is the command list for the palette: one command per
	// line, its name and its help separated by StatusCommandSeparator.
	StatusFieldCommands = "commands"
	// StatusFieldHealthy is "true" when the program answered its health check.
	StatusFieldHealthy = "healthy"
	// StatusFieldContextTokens is how many tokens the last model call held in
	// its context, which the header measures against the window.
	StatusFieldContextTokens = "contextTokens"
	// StatusFieldContextWindow is how many tokens the model can hold on one
	// call, from the model's ContextLength.
	StatusFieldContextWindow = "contextWindow"
	// StatusFieldStreamed is how many tokens the call in progress has written
	// so far, sent while the state is thinking so the screen can show that the
	// model is alive.
	StatusFieldStreamed = "streamed"
	// StatusFieldCallStarted is when the call in progress began, written in
	// RFC 3339 form, so the screen can count the seconds beside the spinner.
	StatusFieldCallStarted = "callStarted"
	// StatusFieldRecordLine is one line for the latest change to the record: a
	// task or a job created, started, finished, or failed, with its id, which
	// the screen shows once as a pill when the line changes.
	StatusFieldRecordLine = "recordLine"
	// StatusFieldJob is the number of the job the running task belongs to,
	// such as "4", and is sent empty when the running task is a person's or
	// nothing is running, so that a screen showing a job knows when to stop.
	StatusFieldJob = "job"
	// StatusFieldJobAsk is that job's ask, folded onto one line.
	StatusFieldJobAsk = "jobAsk"
	// StatusFieldTaskAsk is the running task's ask, folded onto one line, so
	// that a screen can say what a plain task is beside its number. It is
	// empty when nothing is running.
	StatusFieldTaskAsk = "taskAsk"
	// StatusFieldJobName is the short name the model gave the job, such as
	// "Tater Tots Tetris", which a screen shows in place of the ask. It is empty
	// on a job made without a name, and a screen then falls back to the ask.
	StatusFieldJobName = "jobName"
	// StatusFieldJobTask is the label of the job's task that is running now,
	// such as "t31", so that a screen can point at it in the list.
	StatusFieldJobTask = "jobTask"
	// StatusFieldJobTasks is the job's task list as JobTaskLines writes it: one
	// task per line, each beginning with "[x] " when the task is done and "[ ] "
	// when it is not, then the task's label and its text, so that a screen can
	// draw the list with a check beside every task that is finished.
	StatusFieldJobTasks = "jobTasks"
	// StatusFieldPlan is the running task's plan, one done-when step per line,
	// each beginning with "[x] " when the step is done and "[ ] " when it is
	// not, so a screen can draw how far through its plan a task is. It is empty
	// when no task is running or the task has no plan yet.
	StatusFieldPlan = "plan"
	// StatusFieldJobs is how many jobs are waiting, written as a number, so a
	// screen can say there is work queued beyond the running task. It is empty
	// when none are waiting.
	StatusFieldJobs = "jobs"
	// StatusFieldSituation is the running task's situation as the record
	// holds it: one fact per line, in the record's own words, such as "tests:
	// all 51 passing" and "last command: npm test, exit 0". Empty when no task
	// is running or its record cannot be read.
	StatusFieldSituation = "situation"
	// StatusFieldFailures is the running task's failures from the record's
	// lessons, one per line as "F2 <what went wrong> Cause: <why>", newest
	// last. Empty when there are none.
	StatusFieldFailures = "failures"
	// StatusFieldCachedTokens is how many of the last model call's input tokens
	// the provider read from its cache, written as a number, so a screen can
	// show the share of the prompt that was reused.
	StatusFieldCachedTokens = "cachedTokens"
	// StatusFieldRound is the running task's round number, written as a
	// number: how many model calls it has made.
	StatusFieldRound = "round"
	// StatusFieldTaskStarted is when the running task began, written the RFC
	// 3339 way, so a screen can show how long it has run.
	StatusFieldTaskStarted = "taskStarted"
	// ReplyLabel is what a done line names as its result when the answer to
	// the user is its own proof. The harness writes that answer into the record
	// as a result of its own and points the line at it.
	ReplyLabel = "reply"
)

// The two marks a task carries in the job's task list on the wire.
const (
	jobTaskDoneMark = "[x] "
	jobTaskToDoMark = "[ ] "
)

// JobTaskLines writes a job's task list the way StatusFieldJobTasks carries
// it: one task per line, its mark, its label, and its text folded onto the one
// line. The program writes it and a screen reads it back with
// ParseJobTaskLines, so that the two never disagree about the shape.
func JobTaskLines(tasks []JobTask) string {
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		mark := jobTaskToDoMark
		if task.Done {
			mark = jobTaskDoneMark
		}
		lines = append(lines, mark+strings.Join(strings.Fields(task.TaskID+" "+task.Text), " "))
	}
	return strings.Join(lines, "\n")
}

// ParseJobTaskLines reads the task list back: the mark says whether the task
// is done, the first word is its label when it is one a job writes, and the
// rest is its text. A line in any other shape is read as a task that is not
// done, because a list a screen cannot quite read is still a list worth
// showing. Only what the list carries comes back: the report behind a finished
// task and the date a waiting one is due stay in the record.
func ParseJobTaskLines(text string) []JobTask {
	tasks := []JobTask{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		task := JobTask{}
		if rest, marked := strings.CutPrefix(line, jobTaskDoneMark); marked {
			task.Done, line = true, rest
		} else if rest, marked := strings.CutPrefix(line, jobTaskToDoMark); marked {
			line = rest
		}
		words := strings.Fields(line)
		if len(words) > 0 {
			if _, isALabel := ParseTaskID(words[0]); isALabel {
				task.TaskID, words = words[0], words[1:]
			}
		}
		task.Text = strings.Join(words, " ")
		tasks = append(tasks, task)
	}
	return tasks
}

// StatusCommandSeparator separates a command's name from its help line inside
// the commands field.
const StatusCommandSeparator = "\t"

// The words a status envelope's state field may carry, which the screen turns
// into what it says in its status strip.
const (
	// StateIdle means nothing is running.
	StateIdle = "idle"
	// StateThinking means a model call is in flight.
	StateThinking = "thinking"
	// StateUsingTool means a tool is running.
	StateUsingTool = "using"
	// StateWaitingForYou means a preview or a question is waiting.
	StateWaitingForYou = "waiting"
	// StatePaused means scheduled work is paused.
	StatePaused = "paused"
)

// KnownScreenState says whether the word is one of the five.
func KnownScreenState(state string) bool {
	switch state {
	case StateIdle, StateThinking, StateUsingTool, StateWaitingForYou, StatePaused:
		return true
	default:
		return false
	}
}

// ErrEmptySocketLine means the caller handed the decoder a blank line.
var ErrEmptySocketLine = errors.New("the socket line was empty, so there is no message to read")

// EncodeSocketEnvelope writes one envelope as a single line of JSON with a
// newline after it, which is the whole of the socket's framing.
func EncodeSocketEnvelope(writer io.Writer, envelope SocketEnvelope) error {
	line, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("cannot write the socket message as JSON: %w", err)
	}
	// The JSON encoder escapes every newline inside a string, so the only
	// newline in the line is the one added here as the frame.
	if _, err := writer.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("cannot send the socket message: %w", err)
	}
	return nil
}

// DecodeSocketEnvelope reads one line from the socket. It refuses a line that is
// not a JSON object, and a message whose type belongs to neither side, so that a
// screen and the program can never quietly disagree about what was said.
func DecodeSocketEnvelope(line []byte) (SocketEnvelope, error) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return SocketEnvelope{}, ErrEmptySocketLine
	}
	var envelope SocketEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return SocketEnvelope{}, fmt.Errorf("cannot read the socket line as a JSON object: %w", err)
	}
	if !envelope.Type.FromScreen() && !envelope.Type.FromProgram() {
		return SocketEnvelope{}, fmt.Errorf("the socket message type %q is not one either side sends, so check the sender's version", envelope.Type)
	}
	return envelope, nil
}
