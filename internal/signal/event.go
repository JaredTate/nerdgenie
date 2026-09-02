// The rules for reading one event were borrowed from OpenClaw's Signal event
// handler at ~/Code/openclaw/extensions/signal/src/monitor/event-handler.ts and
// from ZeroClaw's Signal channel at
// ~/Code/zeroclaw/crates/zeroclaw-channels/src/signal.rs. The field names were
// read from signal-cli 0.13.23 itself, whose JsonAttachment record carries
// contentType, filename, id and size, and whose receive handler wraps every
// envelope in an object with an account beside it. The Go here is written fresh.

package signal

import "encoding/json"

// MaxEventBytes is the largest event payload the decoder will look at. One
// inbound message is a few hundred bytes, so a megabyte is far more than any
// real event needs and small enough that a daemon gone wrong cannot fill memory.
const MaxEventBytes = 1 << 20

// Attachment is one file or photo that came with an inbound message. The
// identifier is opaque: the bytes are fetched from the daemon by asking for it.
// The tags are the names signal-cli writes.
type Attachment struct {
	// ID is what the daemon calls the file, and what the download asks for.
	ID string `json:"id"`
	// Filename is the name a person would recognise, which may be empty.
	Filename string `json:"filename"`
	// ContentType is what kind of file it is, which may be empty.
	ContentType string `json:"contentType"`
	// Size is how many bytes the sender said it holds, or zero when unsaid.
	Size int64 `json:"size"`
}

// Event is one inbound message the daemon reported. Only messages a person sent
// become events; receipts, typing notifications, and the daemon's own sync
// messages are dropped before one is built.
type Event struct {
	// Account is the Signal account that received the message.
	Account string
	// Sender is the person who sent it, as a phone number or an account
	// identifier, in whatever form the daemon reported.
	Sender string
	// SenderName is the name the sender's profile shows, which may be empty.
	SenderName string
	// Timestamp is when Signal says the message was sent, in milliseconds.
	Timestamp int64
	// Text is what the sender wrote, which is empty for a photo with no caption.
	Text string
	// Attachments are the files and photos that came with it.
	Attachments []Attachment
	// GroupID names the group it was sent to, and is empty for a direct message.
	GroupID string
}

// wireEvent is the outer object signal-cli writes on its event stream: the
// account that received something, and either an envelope or an exception.
type wireEvent struct {
	// Account is the account the daemon received the event for.
	Account string `json:"account"`
	// Envelope is the event itself, left unread until the sync-message check has
	// looked at which keys are there.
	Envelope json.RawMessage `json:"envelope"`
}

// wireEnvelope is one envelope, which may hold a data message, an edited
// message, or none of the two.
type wireEnvelope struct {
	// Source is the sender in whatever form the daemon had to hand.
	Source string `json:"source"`
	// SourceNumber is the sender's phone number, and is preferred.
	SourceNumber string `json:"sourceNumber"`
	// SourceUUID is the sender's account identifier, used when there is no
	// number.
	SourceUUID string `json:"sourceUuid"`
	// SourceName is the name on the sender's profile.
	SourceName string `json:"sourceName"`
	// Timestamp is when the message was sent, in milliseconds.
	Timestamp int64 `json:"timestamp"`
	// DataMessage is the body of an ordinary message.
	DataMessage *wireDataMessage `json:"dataMessage"`
	// EditMessage holds the body when the sender edited a message they had
	// already sent.
	EditMessage *wireEditMessage `json:"editMessage"`
}

// wireEditMessage carries the new body of a message the sender edited.
type wireEditMessage struct {
	// DataMessage is the edited body, in the same shape as an ordinary one.
	DataMessage *wireDataMessage `json:"dataMessage"`
}

// wireDataMessage is the body of one message: the words, the files, and the
// group it belongs to.
type wireDataMessage struct {
	// Message is what the sender wrote.
	Message string `json:"message"`
	// Timestamp is when the body was written, in milliseconds.
	Timestamp int64 `json:"timestamp"`
	// Attachments are the files and photos that came with it.
	Attachments []Attachment `json:"attachments"`
	// GroupInfo names the group, when there is one.
	GroupInfo *wireGroupInfo `json:"groupInfo"`
}

// wireGroupInfo names the group a message was sent to.
type wireGroupInfo struct {
	// GroupID is the group's identifier.
	GroupID string `json:"groupId"`
}

// DecodeEvent reads one event payload from the daemon's stream and returns the
// message in it. The second result is false when the payload is not a message a
// person sent, which covers a delivery receipt, a typing notification, the
// daemon's own sync of what this account sent elsewhere, an event with no
// sender, an empty message, anything that will not parse, and anything past the
// cap. It never panics, whatever bytes it is handed.
func DecodeEvent(payload []byte) (Event, bool) {
	if len(payload) == 0 || len(payload) > MaxEventBytes {
		return Event{}, false
	}

	var outer wireEvent
	if err := json.Unmarshal(payload, &outer); err != nil || len(outer.Envelope) == 0 {
		return Event{}, false
	}
	if isSyncMessage(outer.Envelope) {
		return Event{}, false
	}

	var envelope wireEnvelope
	if err := json.Unmarshal(outer.Envelope, &envelope); err != nil {
		return Event{}, false
	}
	body := envelope.DataMessage
	if body == nil && envelope.EditMessage != nil {
		body = envelope.EditMessage.DataMessage
	}
	sender := firstNonEmpty(envelope.SourceNumber, envelope.SourceUUID, envelope.Source)
	if body == nil || sender == "" {
		return Event{}, false
	}

	event := Event{
		Account:     outer.Account,
		Sender:      sender,
		SenderName:  envelope.SourceName,
		Timestamp:   firstNonZero(envelope.Timestamp, body.Timestamp),
		Text:        body.Message,
		Attachments: keepAttachmentsWithIdentifiers(body.Attachments),
	}
	if body.GroupInfo != nil {
		event.GroupID = body.GroupInfo.GroupID
	}
	if event.Text == "" && len(event.Attachments) == 0 {
		return Event{}, false
	}
	return event, true
}

// isSyncMessage says whether the envelope carries the account's own record of
// what it sent from another device. The check is for the key being there at all,
// because signal-cli writes the key with a null under it rather than leaving it
// out, and a message the agent already sent must never come back as a new one.
func isSyncMessage(envelope json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(envelope, &fields); err != nil {
		return false
	}
	_, present := fields["syncMessage"]
	return present
}

// keepAttachmentsWithIdentifiers drops any attachment with no identifier,
// because the identifier is the only way to fetch the bytes.
func keepAttachmentsWithIdentifiers(reported []Attachment) []Attachment {
	found := make([]Attachment, 0, len(reported))
	for _, one := range reported {
		if one.ID != "" {
			found = append(found, one)
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

// firstNonEmpty returns the first of the choices that says something.
func firstNonEmpty(choices ...string) string {
	for _, choice := range choices {
		if choice != "" {
			return choice
		}
	}
	return ""
}

// firstNonZero returns the first of the numbers that is not zero.
func firstNonZero(numbers ...int64) int64 {
	for _, number := range numbers {
		if number != 0 {
			return number
		}
	}
	return 0
}
