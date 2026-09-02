package signal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// saveIntoInbox downloads the files that came with one message and puts them in
// the home's inbox, where the model can read them, returning their paths. A file
// that will not download is left out rather than holding the message up, because
// the words the sender wrote matter more than the picture.
func (channel *Channel) saveIntoInbox(ctx context.Context, event Event) []string {
	if len(event.Attachments) == 0 {
		return nil
	}
	landed := []string{}
	for at, attachment := range event.Attachments {
		if at >= MaxAttachmentsPerMessage {
			break
		}
		cached, err := channel.client.Download(ctx, attachment, event.Sender)
		if err != nil {
			continue
		}
		path, err := copyIntoInbox(cached, channel.inbox, inboxName(event, attachment))
		if err != nil {
			continue
		}
		landed = append(landed, path)
	}
	if len(landed) == 0 {
		return nil
	}
	return landed
}

// inboxName is what one attachment is called in the inbox: the channel it came
// through, when it arrived, and the name a person would recognise, so that a
// folder of them reads in order and nothing is written over.
func inboxName(event Event, attachment Attachment) string {
	readable := strings.Map(keepPlainNameCharacters, attachment.Filename)
	readable = strings.Trim(readable, ".")
	if readable == "" {
		readable = strings.Map(keepPlainNameCharacters, attachment.ID)
		readable = strings.Trim(readable, ".")
	}
	if readable == "" {
		readable = "attachment"
	}
	if len(readable) > maxCachedNameLength {
		readable = readable[len(readable)-maxCachedNameLength:]
	}
	return fmt.Sprintf("signal-%d-%s", event.Timestamp, readable)
}

// copyIntoInbox copies one downloaded file into the inbox folder under the name
// given, making the folder if it is not there.
func copyIntoInbox(from string, inbox string, name string) (string, error) {
	if err := os.MkdirAll(inbox, contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("cannot make the inbox folder %s, so check that the home folder is writable: %w", inbox, err)
	}
	content, err := os.ReadFile(from)
	if err != nil {
		return "", fmt.Errorf("cannot read the downloaded file %s to put it in the inbox: %w", from, err)
	}
	path := filepath.Join(inbox, name)
	if err := os.WriteFile(path, content, contract.DataFileMode); err != nil {
		return "", fmt.Errorf("cannot write %s into the inbox, so check that the home folder is writable: %w", path, err)
	}
	return path, nil
}
