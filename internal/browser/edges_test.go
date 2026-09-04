package browser

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// A browser built with a piece missing says which piece, because a browser wired
// wrongly must fail where it was wired and not later on a page.
func TestABrowserBuiltWithAPieceMissingSaysWhichPiece(t *testing.T) {
	world := newWorld(t)
	whole := Options{
		Start: world.starts.run, Channel: world.channel,
		Secrets: world.secrets, Clock: world.clock,
	}
	for name, missing := range map[string]func(options *Options){
		"a start function": func(options *Options) { options.Start = nil },
		"a channel":        func(options *Options) { options.Channel = nil },
		"the vault":        func(options *Options) { options.Secrets = nil },
		"a clock":          func(options *Options) { options.Clock = nil },
	} {
		options := whole
		missing(&options)
		if _, err := New(options); err == nil {
			t.Fatalf("a browser built without %s should have been refused", name)
		}
	}

	built, err := New(whole)
	if err != nil {
		t.Fatalf("building a browser with everything failed: %v", err)
	}
	if built.options.HandoffTimeout != DefaultHandoffTimeout ||
		built.options.DailyActionsPerSite != DefaultDailyActionsPerSite ||
		built.options.IdleStop != DefaultIdleStop {
		t.Fatalf("the defaults came out as %s, %d, and %s", built.options.HandoffTimeout,
			built.options.DailyActionsPerSite, built.options.IdleStop)
	}
	built.options.Note("a note with nothing listening: %d", 1)
}

// An answer that carries no identifier, or one from another request, is out of
// step and is reported as such rather than read as this request's answer.
func TestAnAnswerFromAnotherRequestIsOutOfStep(t *testing.T) {
	for _, answer := range []string{
		`{"jsonrpc":"2.0","result":{"healthy":true}}`,
		`{"jsonrpc":"2.0","id":77,"result":{"healthy":true}}`,
		`{"jsonrpc":"2.0","id":1}`,
	} {
		var health contract.BrowserHealth
		err := readAnswer([]byte(answer), 1, &health)
		if err == nil {
			t.Fatalf("the answer %s was read as this request's answer, and it is not", answer)
		}
	}
	if err := readAnswer([]byte("not JSON at all"), 1, nil); err == nil ||
		!strings.Contains(err.Error(), "not JSON at all") {
		t.Fatalf("a line that is not an answer said %v, and it should quote what it could not read", err)
	}
	long := strings.Repeat("a", 400)
	if err := readAnswer([]byte(long), 1, nil); err == nil || !strings.Contains(err.Error(), "...") {
		t.Fatalf("a very long line said %v, and only as much of it as is worth reading belongs in a message", err)
	}
}

// An answer whose result is not the shape the protocol promises is reported as
// that, so that a worker whose JSON has drifted is caught at the wire.
func TestAnAnswerOfTheWrongShapeIsReportedAtTheWire(t *testing.T) {
	var page contract.Snapshot
	err := readAnswer([]byte(`{"jsonrpc":"2.0","id":1,"result":"a piece of text"}`), 1, &page)
	if err == nil || !strings.Contains(err.Error(), "the shape the protocol promises") {
		t.Fatalf("a result of the wrong shape said %v, and it should have said the shape is wrong", err)
	}
}

// A batch that stops part way still hands back the diffs of the steps that ran,
// which is what the protocol promises the model.
func TestABatchThatStopsHandsBackTheStepsThatRan(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheSimplePage(t, browser)

	diffs, err := browser.Act(context.Background(), []contract.ActStep{
		{Method: "type", Ref: testkit.FixtureUsernameRef, Text: "one", Expectation: "the box named Name holds it"},
		{Method: "click", Ref: "e99", Expectation: "something happens"},
	})
	if err == nil {
		t.Fatal("a batch whose second step points at nothing should have been refused")
	}
	if len(diffs) != 1 {
		t.Fatalf("the batch handed back %d diffs, and the first step did run", len(diffs))
	}
	if _, err := browser.Act(context.Background(), nil); err == nil {
		t.Fatal("a batch with no steps in it should have been refused")
	}
}

// A worker that says it is unhealthy but will not say why is still reported with
// something a person can act on.
func TestAWorkerThatWillNotSayWhySaysWhatToCheck(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) {
		options.Start = world.startedBy(scriptedStart(func(line []byte) string {
			return `{"jsonrpc":"2.0","id":` + identifierIn(line) + `,"result":{"healthy":false}}` + "\n"
		}))
	})
	_, err := browser.Health(context.Background())
	if err == nil || !strings.Contains(err.Error(), "google-chrome is installed") {
		t.Fatalf("a worker that would not say why said %v, and it should say what to check", err)
	}
}

// A worker that goes away in the middle of a call is reported as an
// interruption, which is what the protocol's table says of a browser that has
// gone.
func TestAWorkerThatGoesAwayMidCallIsAnInterruption(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, func(options *Options) {
		options.Start = world.startedBy(scriptedStart(healthyThen(func([]byte) string { return "" })))
	})
	if _, err := browser.Health(context.Background()); err != nil {
		t.Fatalf("the health check on a scripted worker failed: %v", err)
	}

	browser.guard.Lock()
	_ = browser.connection.Requests.Close()
	browser.guard.Unlock()
	_, err := browser.Read(context.Background(), contract.ReadOptions{})
	if err == nil || !strings.Contains(err.Error(), "the browser was interrupted") {
		t.Fatalf("a worker that went away said %v, and it should be reported as an interruption", err)
	}
}

// The message that goes with the picture lists only as many marks as a person
// will read, and says how many more there were.
func TestTheMessageListsOnlyAsManyMarksAsAPersonWillRead(t *testing.T) {
	marks := make([]contract.Mark, 0, mostMarksListed+5)
	for number := 1; number <= mostMarksListed+5; number++ {
		marks = append(marks, contract.Mark{Number: number, Ref: "e" + strconv.Itoa(number), Role: "button", Name: "Button"})
	}
	listed := markList(marks)
	if !strings.Contains(listed, "and 5 more") {
		t.Fatalf("the list said %q, and it should have said how many more there were", listed)
	}
	if strings.Contains(listed, strconv.Itoa(mostMarksListed+1)+". the button") {
		t.Fatalf("the list said %q, and it should stop at %d", listed, mostMarksListed)
	}
	if markList(nil) != "" {
		t.Fatalf("a page with no marks on it listed %q, and it should list nothing", markList(nil))
	}
}

// A code the user sends with no box on the page to type it into says so rather
// than typing it somewhere else.
func TestACodeWithNoBoxToTypeItIntoSaysSo(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	if _, err := browser.Open(context.Background(), testkit.FixtureCaptchaPage); err != nil {
		t.Fatalf("opening the captcha page failed: %v", err)
	}
	pushMessage(t, world, "748291")

	_, err := browser.Handoff(context.Background(), "the site is asking whether I am a person")
	if err == nil || !strings.Contains(err.Error(), "no box on the page") {
		t.Fatalf("a code with nowhere to go said %v, and it should have said there is no box for it", err)
	}
}

// The code box is found by its name when the wall named no element of its own.
func TestTheCodeBoxIsFoundByItsNameWhenTheWallNamedNothing(t *testing.T) {
	page := contract.Snapshot{
		Wall: &contract.Wall{Kind: contract.WallTwoFactor, Detail: "the page asks for two factor"},
		Elements: []contract.Element{
			{Ref: "e1", Role: "button", Name: "One time code"},
			{Ref: "e2", Role: "textbox", Name: "One time code"},
		},
	}
	if found := codeBoxOn(page); found != "e2" {
		t.Fatalf("the code box was read as %q, and only a box a person types into counts", found)
	}
	if found := codeBoxOn(contract.Snapshot{}); found != "" {
		t.Fatalf("a page with nothing on it has no code box, and this found %q", found)
	}
}

// A handoff still sends the picture when the page will not read and when the
// window will not come forward, because the picture is what the user needs.
func TestAHandoffStillSendsThePictureWhenTheWindowWillNotBehave(t *testing.T) {
	for name, wanted := range map[string]string{
		"read": "could not be read before the handoff",
		"tabs": "would not come forward for the handoff",
	} {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			refusedMethod := name
			browser := world.browser(t, func(options *Options) {
				options.Start = world.startedBy(scriptedStart(healthyThen(func(line []byte) string {
					return theHandoffScript(line, refusedMethod)
				})))
			})
			pushMessage(t, world, "done")

			reply, err := browser.Handoff(context.Background(), "the site is asking whether I am a person")
			if err != nil {
				t.Fatalf("the handoff failed: %v", err)
			}
			if reply.Kind != HandoffDone {
				t.Fatalf("the handoff came back as %q, and the user said done", reply.Kind)
			}
			if !strings.Contains(world.notes.written(), wanted) {
				t.Fatalf("the log says %q, and it should say %q", world.notes.written(), wanted)
			}
		})
	}
}

// theHandoffScript answers a handoff's three calls, refusing the one method the
// test named.
func theHandoffScript(line []byte, refusedMethod string) string {
	identifier := identifierIn(line)
	method := "read"
	if strings.Contains(string(line), `"method":"tabs"`) {
		method = "tabs"
	} else if strings.Contains(string(line), `"method":"screenshot"`) {
		method = "screenshot"
	}
	if method == refusedMethod {
		return `{"jsonrpc":"2.0","id":` + identifier +
			`,"error":{"code":-32602,"message":"that will not work here, so ask for something else"}}` + "\n"
	}
	switch method {
	case "tabs":
		return `{"jsonrpc":"2.0","id":` + identifier + `,"result":{"tabs":[{"id":"t1","url":"https://a.test/"}]}}` + "\n"
	case "screenshot":
		return `{"jsonrpc":"2.0","id":` + identifier + `,"result":{"pngBase64":"","marks":[]}}` + "\n"
	default:
		return `{"jsonrpc":"2.0","id":` + identifier +
			`,"result":{"url":"https://a.test/","title":"A page","tabId":"t1","elements":[],"belowFold":0}}` + "\n"
	}
}

// A handoff whose task is called off comes back with the context's own reason
// rather than pretending the user answered.
func TestAHandoffThatIsCalledOffSaysSo(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheCaptchaPage(t, browser)

	ctx, callOff := context.WithCancel(context.Background())
	answered := make(chan error, 1)
	go func() {
		_, err := browser.Handoff(ctx, "the site is asking whether I am a person")
		answered <- err
	}()
	waitUntil(t, "the handoff to send its picture", func() bool { return len(world.channel.Files()) == 1 })
	callOff()

	if err := <-answered; err == nil {
		t.Fatal("a handoff whose task was called off should have said so rather than answering")
	}
}

// A channel that goes away while the browser is waiting ends the handoff rather
// than holding the task for ever.
func TestAChannelThatGoesAwayEndsTheHandoff(t *testing.T) {
	world := newWorld(t)
	browser := world.browser(t, nil)
	openTheCaptchaPage(t, browser)

	answered := make(chan error, 1)
	go func() {
		_, err := browser.Handoff(context.Background(), "the site is asking whether I am a person")
		answered <- err
	}()
	waitUntil(t, "the handoff to send its picture", func() bool { return len(world.channel.Files()) == 1 })
	world.channel.Shutdown()

	select {
	case err := <-answered:
		if err == nil || !strings.Contains(err.Error(), "the channel closed") {
			t.Fatalf("a channel that went away said %v, and it should say the channel closed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a channel that went away should have ended the handoff, and the task is still waiting")
	}
}
