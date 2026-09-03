package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
)

// FromScreen says whether a screen sends this kind of message.
func (kind SocketMessageType) FromScreen() bool {
	switch kind {
	case SocketMessage, SocketCommand, SocketApprove, SocketDeny,
		SocketSecret, SocketAttach, SocketDetach:
		return true
	default:
		return false
	}
}

// FromProgram says whether the program sends this kind of message.
func (kind SocketMessageType) FromProgram() bool {
	switch kind {
	case SocketDelta, SocketReply, SocketPreview, SocketAsk,
		SocketHandoff, SocketStatus, SocketError:
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
)

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
