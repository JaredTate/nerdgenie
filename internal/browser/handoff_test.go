package browser

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// A handoff sends the user a numbered picture of the page and holds the task
// until they say they have finished.
func TestAHandoffSendsThePictureAndWaitsForDone(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheCaptchaPage(t, browser)
	pushMessage(t, world, "done")

	reply, err := browser.Handoff(context.Background(), "the site is asking whether I am a person")
	if err != nil {
		t.Fatalf("the handoff failed: %v", err)
	}
	if reply.Kind != HandoffDone {
		t.Fatalf("the handoff came back as %q, and the user said done", reply.Kind)
	}
	files := world.channel.Files()
	if len(files) != 1 {
		t.Fatalf("%d files were sent, and a handoff sends one picture", len(files))
	}
	if _, err := os.Stat(files[0].Path); err == nil {
		t.Fatalf("the picture is still at %s, and a handoff takes its file away afterwards", files[0].Path)
	}
	for _, wanted := range []string{"asking whether I am a person", "captcha wall", "1. done", "2. the code", "3. abort"} {
		if !strings.Contains(files[0].Caption, wanted) {
			t.Fatalf("the message with the picture said %q, and it should have said %q", files[0].Caption, wanted)
		}
	}
}

// A code the user replies with is typed into the box the wall named, and never
// comes back to the model.
func TestACodeTheUserSendsIsTypedIntoTheBoxTheWallNamed(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	if _, err := browser.Open(context.Background(), testkit.FixtureTwoFactorPage); err != nil {
		t.Fatalf("opening the two-factor page failed: %v", err)
	}
	pushMessage(t, world, " 748291 ")

	reply, err := browser.Handoff(context.Background(), "the site wants the code from your authenticator")
	if err != nil {
		t.Fatalf("the handoff failed: %v", err)
	}
	if reply.Kind != HandoffCode {
		t.Fatalf("the handoff came back as %q, and the user sent a code", reply.Kind)
	}
	if strings.Contains(reply.Note, "748291") {
		t.Fatalf("the reply says %q, and a code the user sent is typed into the page and never handed to the model", reply.Note)
	}
	if typed := world.worker.TypedInto("e5"); len(typed) != 1 || typed[0] != "748291" {
		t.Fatalf("the worker typed %v into the code box, and it should have been the code the user sent", typed)
	}
}

// The user can tell the agent to give up.
func TestTheUserCanTellTheAgentToGiveUp(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheCaptchaPage(t, browser)
	pushMessage(t, world, "ABORT")

	reply, err := browser.Handoff(context.Background(), "the site is asking whether I am a person")
	if err != nil {
		t.Fatalf("the handoff failed: %v", err)
	}
	if reply.Kind != HandoffAbort || !strings.Contains(reply.Note, "give up") {
		t.Fatalf("the handoff came back as %+v, and the user said abort", reply)
	}
}

// Anything that is none of the three answers is answered with the three choices
// again rather than taken for one of them.
func TestAnAnswerThatIsNoneOfTheThreeIsAnsweredWithTheChoices(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheCaptchaPage(t, browser)
	pushMessage(t, world, "12345678901")
	pushMessage(t, world, "done")

	reply, err := browser.Handoff(context.Background(), "the site is asking whether I am a person")
	if err != nil {
		t.Fatalf("the handoff failed: %v", err)
	}
	if reply.Kind != HandoffDone {
		t.Fatalf("the handoff came back as %q, and the second message said done", reply.Kind)
	}
	sent := world.channel.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "one of these three") {
		t.Fatalf("the user was told %v, and a message that is none of the three gets the three choices back", sent)
	}
}

// A handoff nobody answers gives up when the configured timeout passes, and says
// so plainly rather than failing.
func TestAHandoffNobodyAnswersTimesOut(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheCaptchaPage(t, browser)

	answered := make(chan HandoffReply, 1)
	go func() {
		reply, err := browser.Handoff(context.Background(), "the site is asking whether I am a person")
		if err != nil {
			t.Errorf("the handoff that nobody answered failed: %v", err)
		}
		answered <- reply
	}()
	waitForSleepers(t, world.clock, 2)
	world.clock.Advance(2 * theHandoffWait)

	reply := <-answered
	if reply.Kind != HandoffTimedOut || !strings.Contains(reply.Note, "1m0s") {
		t.Fatalf("the handoff came back as %+v, and it should have said nobody answered within the timeout", reply)
	}
}

// The command sends a picture of the browser through the channel it was typed
// on.
func TestTheScreenCommandSendsAPictureThroughItsOwnChannel(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)
	command := ScreenCommand(browser)

	if command.Name != "screen" || command.Help == "" {
		t.Fatalf("the command is %+v, and it should be called screen and have a help line", command)
	}
	said, err := command.Run(context.Background(), "", contract.CommandContext{Channel: world.channel})
	if err != nil {
		t.Fatalf("the screen command failed: %v", err)
	}
	files := world.channel.Files()
	if len(files) != 1 || !strings.Contains(files[0].Caption, testkit.FixtureSimplePage) {
		t.Fatalf("%d files were sent with the caption %q, and it should be one picture of the fixture page", len(files), captionOf(files))
	}
	if !strings.Contains(said, testkit.FixtureSimplePage) {
		t.Fatalf("the command said %q, and it should have said where the browser is", said)
	}
	if _, err := command.Run(context.Background(), "", contract.CommandContext{}); err == nil {
		t.Fatal("the screen command with nowhere to send the picture should have said so")
	}
}

// A picture larger than the cap is not sent at all.
func TestAPictureLargerThanTheCapIsNotSent(t *testing.T) {
	if _, err := savePicture(strings.Repeat("a", mostPictureBytes+1)); err == nil {
		t.Fatal("a picture larger than the cap should have been refused")
	}
	if _, err := savePicture("this is not base64 at all!!"); err == nil {
		t.Fatal("a picture that is not the base64 text the protocol promises should have been refused")
	}
}

// The three answers are read the way a person would write them, and nothing else
// is taken for one of them.
func TestTheThreeAnswersAreReadTheWayAPersonWritesThem(t *testing.T) {
	for text, wanted := range map[string]HandoffKind{
		"done": HandoffDone, " DONE ": HandoffDone, "abort": HandoffAbort,
		"1234": HandoffCode, "748291": HandoffCode,
		// On 8 September 2026 the person typed "its there" and was told three
		// times that it was not understood; a person who writes anything but
		// abort or a code after a handoff has done what was asked.
		"its there": HandoffDone, "yes": HandoffDone, "done please": HandoffDone, "ok go": HandoffDone, "12a456": HandoffDone,
	} {
		if got, understood := readHandoffAnswer(text); !understood || got != wanted {
			t.Fatalf("%q was read as %q, and it should have been %q", text, got, wanted)
		}
	}
	for _, text := range []string{"", "   ", "12", "12345678901"} {
		if got, understood := readHandoffAnswer(text); understood {
			t.Fatalf("%q was read as %q, and it is none of the three answers", text, got)
		}
	}
}

// theHandoffWait is the timeout every handoff test is built with.
const theHandoffWait = time.Minute

// openTheCaptchaPage puts the browser on the wall the handoff tests hand off
// from.
func openTheCaptchaPage(t *testing.T, browser *Browser) {
	t.Helper()
	if _, err := browser.Open(context.Background(), testkit.FixtureCaptchaPage); err != nil {
		t.Fatalf("opening the captcha page failed: %v", err)
	}
}

// pushMessage puts one message into the channel as though the user had sent it.
func pushMessage(t *testing.T, world *world, text string) {
	t.Helper()
	if err := world.channel.Push(contract.Inbound{ID: text, Sender: "jared", Text: text}); err != nil {
		t.Fatalf("pushing %q into the channel failed: %v", text, err)
	}
}

// captionOf is the caption of the first file sent, for an error message.
func captionOf(files []testkit.SentFile) string {
	if len(files) == 0 {
		return ""
	}
	return files[0].Caption
}
