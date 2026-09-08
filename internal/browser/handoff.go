package browser

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The bounds a handoff keeps.
const (
	// mostPictureBytes caps the picture the worker sends back, because the
	// picture is decoded into memory before it is written to a file.
	mostPictureBytes = 8 << 20
	// mostMarksListed caps how many numbered marks the message lists, so that a
	// crowded page does not turn into a message nobody will read.
	mostMarksListed = 20
	// mostHandoffMessages caps how many messages a handoff will read before it
	// gives up, so that a chatty channel cannot hold the task for ever.
	mostHandoffMessages = 20
	// shortestCode and longestCode bound what counts as a code typed by the user
	// rather than a word.
	shortestCode = 4
	longestCode  = 10
)

// HandoffKind is what the user did when the browser was handed to them.
type HandoffKind string

const (
	// HandoffDone means the user finished at the browser and the agent may carry
	// on.
	HandoffDone HandoffKind = "done"
	// HandoffCode means the user sent a code, which was typed into the box the
	// wall named.
	HandoffCode HandoffKind = "code"
	// HandoffAbort means the user wants the agent to give up.
	HandoffAbort HandoffKind = "abort"
	// HandoffTimedOut means nobody answered before the handoff timeout passed.
	HandoffTimedOut HandoffKind = "timeout"
)

// HandoffReply is what came back from the user. It never carries the code
// itself: the code was typed into the page and telling the model what it was
// would put a secret in its context for nothing.
type HandoffReply struct {
	// Kind is which of the four things happened.
	Kind HandoffKind
	// Note is one sentence for the model saying what happened.
	Note string
}

// Handoff brings the browser window forward, sends the user a numbered picture
// of the page through the current channel, and holds the task until they reply
// that they are done, send a code, or tell the agent to give up. Design section
// 9 says the agent never tries to get past a login wall, a prompt for a second
// code, or a captcha; this is what it does instead.
func (browser *Browser) Handoff(ctx context.Context, reason string) (HandoffReply, error) {
	page := browser.bringWindowForward(ctx)
	picture, err := browser.Screenshot(ctx)
	if err != nil {
		return HandoffReply{}, fmt.Errorf("the browser could not be handed to the user because it could not be photographed: %w", err)
	}
	pictureFile, err := savePicture(picture.PNGBase64)
	if err != nil {
		return HandoffReply{}, err
	}
	defer func() { _ = os.Remove(pictureFile) }()

	caption := handoffMessage(reason, page, picture.Marks, browser.options.HandoffTimeout.String())
	if err := browser.options.Channel.SendFile(ctx, pictureFile, caption); err != nil {
		return HandoffReply{}, fmt.Errorf("the browser could not be handed to the user because the picture would not send: %w", err)
	}
	browser.options.Note("the browser was handed to the user at %s because %s", page.URL, reason)
	return browser.waitForTheUser(ctx, page)
}

// bringWindowForward puts the tab the agent is acting on in front of the user,
// which the worker does when it is asked to switch to a tab. A window that will
// not come forward is noted and the handoff carries on, because the picture is
// worth more than the window.
func (browser *Browser) bringWindowForward(ctx context.Context) contract.Snapshot {
	page, err := browser.Read(ctx, contract.ReadOptions{})
	if err != nil {
		browser.options.Note("the page could not be read before the handoff: %v", err)
		return contract.Snapshot{}
	}
	if _, err := browser.Tabs(ctx, contract.TabSwitch, page.TabID); err != nil {
		browser.options.Note("the browser window would not come forward for the handoff: %v", err)
	}
	return page
}

// savePicture writes the picture to a file of its own for the channel to send,
// readable by nobody else, and refuses one larger than the cap.
func savePicture(encoded string) (string, error) {
	if len(encoded) > mostPictureBytes {
		return "", fmt.Errorf("the picture of the page is larger than the cap of %d bytes, so it was not sent", mostPictureBytes)
	}
	picture, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("the picture of the page was not the base64 text the protocol promises: %w", err)
	}
	file, err := os.CreateTemp("", "nerdgenie-browser-*.png")
	if err != nil {
		return "", fmt.Errorf("a file for the picture of the page could not be made: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err := file.Chmod(0o600); err != nil {
		return "", fmt.Errorf("the picture of the page could not be made private: %w", err)
	}
	if _, err := file.Write(picture); err != nil {
		return "", fmt.Errorf("the picture of the page could not be written to %s: %w", file.Name(), err)
	}
	return file.Name(), nil
}

// waitForTheUser holds the task until the user answers or the handoff timeout
// passes. A message that is none of the three answers is answered with the three
// choices again rather than being taken as one of them.
func (browser *Browser) waitForTheUser(ctx context.Context, page contract.Snapshot) (HandoffReply, error) {
	waitCtx, stopWaiting := context.WithCancel(ctx)
	defer stopWaiting()
	messages, err := browser.options.Channel.Receive(waitCtx)
	if err != nil {
		return HandoffReply{}, fmt.Errorf("the browser was handed to the user but their answer could not be listened for: %w", err)
	}
	timedOut := browser.afterTheHandoffTimeout(waitCtx)

	for read := 0; read < mostHandoffMessages; read++ {
		select {
		case message, open := <-messages:
			if !open {
				return HandoffReply{}, errors.New("the channel closed while the browser was waiting for the user, so the handoff was given up")
			}
			reply, understood := readHandoffAnswer(message.Text)
			if !understood {
				_ = browser.options.Channel.Send(waitCtx, theThreeChoices)
				continue
			}
			return browser.actOnTheAnswer(ctx, reply, message.Text, page)
		case <-timedOut:
			return browser.nobodyAnswered(), nil
		case <-ctx.Done():
			return HandoffReply{}, ctx.Err()
		}
	}
	return HandoffReply{}, fmt.Errorf("the browser read %d messages at the handoff and none of them said done, a code, or abort, so it gave up", mostHandoffMessages)
}

// nobodyAnswered is the reply when the handoff timeout passed with nothing said.
func (browser *Browser) nobodyAnswered() HandoffReply {
	browser.options.Note("nobody answered the handoff within %s", browser.options.HandoffTimeout)
	return HandoffReply{
		Kind: HandoffTimedOut,
		Note: fmt.Sprintf("nobody answered the browser handoff within %s, so the page is still waiting at the wall", browser.options.HandoffTimeout),
	}
}

// afterTheHandoffTimeout closes its channel once the handoff timeout has really
// passed on the clock, and never when the wait was simply cut short.
func (browser *Browser) afterTheHandoffTimeout(ctx context.Context) <-chan struct{} {
	passed := make(chan struct{})
	go func() {
		if err := browser.options.Clock.Sleep(ctx, browser.options.HandoffTimeout); err == nil {
			close(passed)
		}
	}()
	return passed
}

// actOnTheAnswer does what the user asked. A code is typed into the box the wall
// named and is never returned to the model.
func (browser *Browser) actOnTheAnswer(ctx context.Context, kind HandoffKind, text string,
	page contract.Snapshot) (HandoffReply, error) {
	switch kind {
	case HandoffDone:
		return HandoffReply{Kind: HandoffDone, Note: "the user says they have finished at the browser, so read the page and carry on"}, nil
	case HandoffAbort:
		return HandoffReply{Kind: HandoffAbort, Note: "the user told the agent to give up at the browser, so stop this task and say why"}, nil
	default:
		if err := browser.typeTheCode(ctx, strings.TrimSpace(text), page); err != nil {
			return HandoffReply{}, err
		}
		return HandoffReply{Kind: HandoffCode, Note: "the user sent a code and it was typed into the box on the page, so read the page and carry on"}, nil
	}
}

// typeTheCode types the code the user sent into the box the wall named, reading
// the page again when the page it was handed off from no longer says where that
// box is.
func (browser *Browser) typeTheCode(ctx context.Context, code string, page contract.Snapshot) error {
	ref := codeBoxOn(page)
	if ref == "" {
		if fresh, err := browser.Read(ctx, contract.ReadOptions{}); err == nil {
			ref = codeBoxOn(fresh)
		}
	}
	if ref == "" {
		return errors.New("the user sent a code but there is no box on the page to type it into, so read the page and see what it is asking for")
	}
	if _, err := browser.Type(ctx, ref, code, "the code box holds the code the user sent"); err != nil {
		return fmt.Errorf("the code the user sent could not be typed into the page: %w", err)
	}
	return nil
}

// readHandoffAnswer reads one message as one of the three answers, and says
// whether it was one of them at all. "abort" gives up, a short run of digits
// is the code the site asked for, and any other words are "done": a person
// who writes anything after a handoff has done what was asked, and on 8
// September 2026 one wrote "its there" and was told three times that it was
// not understood. An empty message, or digits that are no code, is neither.
func readHandoffAnswer(text string) (HandoffKind, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	if trimmed == "" {
		return "", false
	}
	if trimmed == "abort" {
		return HandoffAbort, true
	}
	if isAllDigits(trimmed) {
		if looksLikeACode(trimmed) {
			return HandoffCode, true
		}
		return "", false
	}
	return HandoffDone, true
}

// isAllDigits says whether the text is nothing but digits.
func isAllDigits(text string) bool {
	for _, letter := range text {
		if letter < '0' || letter > '9' {
			return false
		}
	}
	return true
}

// looksLikeACode says whether the text is the short run of digits a site asks
// for as a second code, rather than a word.
func looksLikeACode(text string) bool {
	if len(text) < shortestCode || len(text) > longestCode {
		return false
	}
	for _, letter := range text {
		if letter < '0' || letter > '9' {
			return false
		}
	}
	return true
}

// theThreeChoices is what the user is told when they sent something that was
// none of the three answers.
const theThreeChoices = "I did not understand that. Reply with one of these three: " +
	"1. done, when you have finished at the browser. " +
	"2. the code, which is the digits the site asked for, and I will type it in. " +
	"3. abort, and I will give up on this task."

// handoffMessage is the caption that goes with the picture: why the agent
// stopped, where the browser is, what the numbers on the picture point at, and
// the three things the user can reply.
func handoffMessage(reason string, page contract.Snapshot, marks []contract.Mark, waitingFor string) string {
	var built strings.Builder
	fmt.Fprintf(&built, "I need you at the browser: %s\n", strings.TrimSpace(reason))
	fmt.Fprintf(&built, "It is on %q at %s.\n", page.Title, page.URL)
	if page.Wall != nil {
		fmt.Fprintf(&built, "It stopped at a %s wall: %s.\n", page.Wall.Kind, page.Wall.Detail)
	}
	built.WriteString(markList(marks))
	built.WriteString("Reply with one of these three:\n")
	built.WriteString("1. done, when you have finished at the browser and I should carry on.\n")
	built.WriteString("2. the code, which is the digits the site asked for, and I will type it in.\n")
	built.WriteString("3. abort, and I will give up on this task.\n")
	fmt.Fprintf(&built, "I will wait %s.", waitingFor)
	return built.String()
}

// markList is the numbered list of what the marks on the picture point at,
// capped, and empty when the page has none.
func markList(marks []contract.Mark) string {
	if len(marks) == 0 {
		return ""
	}
	var built strings.Builder
	built.WriteString("The numbers on the picture are:\n")
	for at, mark := range marks {
		if at >= mostMarksListed {
			fmt.Fprintf(&built, "and %d more.\n", len(marks)-mostMarksListed)
			break
		}
		fmt.Fprintf(&built, "%d. the %s named %q.\n", mark.Number, mark.Role, mark.Name)
	}
	return built.String()
}
