package signal

import "testing"

func TestDecodeEventReadsADirectMessage(t *testing.T) {
	payload := []byte(`{
		"account": "+15125550100",
		"envelope": {
			"source": "+15125550123",
			"sourceNumber": "+15125550123",
			"sourceUuid": "6f1a5c1e-0000-4000-8000-000000000001",
			"sourceName": "Alice",
			"timestamp": 1700000000000,
			"dataMessage": {"message": "hello there", "timestamp": 1700000000000}
		}
	}`)

	event, isMessage := DecodeEvent(payload)
	if !isMessage {
		t.Fatalf("the decoder did not read a plain direct message as a message")
	}
	if event.Sender != "+15125550123" {
		t.Errorf("the sender is %q, want the source number", event.Sender)
	}
	if event.SenderName != "Alice" {
		t.Errorf("the sender name is %q, want Alice", event.SenderName)
	}
	if event.Text != "hello there" {
		t.Errorf("the text is %q, want the body of the data message", event.Text)
	}
	if event.Timestamp != 1700000000000 {
		t.Errorf("the timestamp is %d, want the envelope's timestamp in milliseconds", event.Timestamp)
	}
	if event.GroupID != "" {
		t.Errorf("the group is %q, want empty for a direct message", event.GroupID)
	}
}

func TestDecodeEventFallsBackToTheSenderIdentifiers(t *testing.T) {
	cases := []struct {
		name     string
		envelope string
		want     string
	}{
		{
			name:     "the source number is preferred",
			envelope: `"sourceNumber": "+15125550123", "sourceUuid": "6f1a5c1e-0000-4000-8000-000000000001", "source": "ignored",`,
			want:     "+15125550123",
		},
		{
			name:     "the account identifier is next",
			envelope: `"sourceUuid": "6f1a5c1e-0000-4000-8000-000000000001", "source": "ignored",`,
			want:     "6f1a5c1e-0000-4000-8000-000000000001",
		},
		{
			name:     "the plain source is last",
			envelope: `"source": "+15125550199",`,
			want:     "+15125550199",
		},
	}

	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			payload := []byte(`{"envelope": {` + oneCase.envelope + `"timestamp": 1, "dataMessage": {"message": "hi"}}}`)
			event, isMessage := DecodeEvent(payload)
			if !isMessage {
				t.Fatalf("the decoder did not read a message with %s", oneCase.name)
			}
			if event.Sender != oneCase.want {
				t.Errorf("the sender is %q, want %q", event.Sender, oneCase.want)
			}
		})
	}
}

func TestDecodeEventDropsWhatIsNotAMessage(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"nothing at all", ``},
		{"not JSON", `{"envelope": `},
		{"no envelope", `{"account": "+15125550100"}`},
		{"an exception instead of an envelope", `{"exception": {"message": "receive failed"}}`},
		{"no sender", `{"envelope": {"timestamp": 1, "dataMessage": {"message": "hi"}}}`},
		{"no data message, which is a receipt", `{"envelope": {"sourceNumber": "+1", "timestamp": 1}}`},
		{"a sync message the daemon reports as null", `{"envelope": {"sourceNumber": "+1", "syncMessage": null, "dataMessage": {"message": "hi"}}}`},
		{"a sync message with a body", `{"envelope": {"sourceNumber": "+1", "syncMessage": {"sentMessage": {}}, "dataMessage": {"message": "hi"}}}`},
		{"a data message with nothing in it", `{"envelope": {"sourceNumber": "+1", "timestamp": 1, "dataMessage": {"message": ""}}}`},
		{"a typing notification", `{"envelope": {"sourceNumber": "+1", "typingMessage": {"action": "STARTED"}}}`},
	}

	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			if _, isMessage := DecodeEvent([]byte(oneCase.payload)); isMessage {
				t.Errorf("the decoder read %s as a message, and only a data message from a real sender is one", oneCase.name)
			}
		})
	}
}

func TestDecodeEventReadsAnEditedMessage(t *testing.T) {
	payload := []byte(`{"envelope": {
		"sourceNumber": "+15125550123",
		"timestamp": 1700000000000,
		"editMessage": {"targetSentTimestamp": 1699999999000, "dataMessage": {"message": "fixed that"}}
	}}`)

	event, isMessage := DecodeEvent(payload)
	if !isMessage {
		t.Fatalf("the decoder did not read an edited message, whose body sits under editMessage")
	}
	if event.Text != "fixed that" {
		t.Errorf("the text is %q, want the body of the edited message", event.Text)
	}
}

func TestDecodeEventReadsAttachmentsAndGroups(t *testing.T) {
	payload := []byte(`{"envelope": {
		"sourceNumber": "+15125550123",
		"timestamp": 0,
		"dataMessage": {
			"message": "",
			"timestamp": 1700000000123,
			"groupInfo": {"groupId": "dGhlIGdyb3Vw", "groupName": "The Group"},
			"attachments": [
				{"id": "abc123", "filename": "photo.jpg", "contentType": "image/jpeg", "size": 4096},
				{"id": "def456"}
			]
		}
	}}`)

	event, isMessage := DecodeEvent(payload)
	if !isMessage {
		t.Fatalf("the decoder dropped a message that carries an attachment and no words")
	}
	if event.GroupID != "dGhlIGdyb3Vw" {
		t.Errorf("the group is %q, want the group identifier from the group information", event.GroupID)
	}
	if event.Timestamp != 1700000000123 {
		t.Errorf("the timestamp is %d, want the data message's own timestamp when the envelope has none", event.Timestamp)
	}
	if len(event.Attachments) != 2 {
		t.Fatalf("the decoder read %d attachments, want both of them", len(event.Attachments))
	}
	first := event.Attachments[0]
	if first.ID != "abc123" || first.Filename != "photo.jpg" || first.ContentType != "image/jpeg" || first.Size != 4096 {
		t.Errorf("the first attachment is %+v, want every field the daemon reported", first)
	}
	if event.Attachments[1].ID != "def456" {
		t.Errorf("the second attachment is %+v, want the one field the daemon reported", event.Attachments[1])
	}
}

func TestDecodeEventRefusesAPayloadPastTheCap(t *testing.T) {
	tooMuch := make([]byte, MaxEventBytes+1)
	for index := range tooMuch {
		tooMuch[index] = ' '
	}
	if _, isMessage := DecodeEvent(tooMuch); isMessage {
		t.Errorf("the decoder read a payload past the cap, and every buffer has a cap")
	}
}
