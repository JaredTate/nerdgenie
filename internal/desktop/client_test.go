package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTheClientSendsOneRequestAndReadsItsAnswer(t *testing.T) {
	worker := newScriptedWorker()
	worker.answer("screenshot", aScreenshot())
	client := newClient(worker.start())

	var picture screenshotAnswer
	if err := client.call(context.Background(), "screenshot", map[string]any{}, &picture); err != nil {
		t.Fatalf("asking for a screenshot failed: %v", err)
	}

	if picture.Application != "zenity" || len(picture.Marks) != 2 {
		t.Errorf("the screenshot came back as %+v, want the fixture window with two marks", picture)
	}
	if asked := worker.methodsAsked(); len(asked) != 1 || asked[0] != "screenshot" {
		t.Errorf("the worker was asked %v, want one screenshot", asked)
	}
}

func TestTheClientNumbersItsRequestsAndMatchesTheAnswers(t *testing.T) {
	worker := newScriptedWorker()
	worker.answer("click", aDiff(true, ""))
	client := newClient(worker.start())

	for round := 0; round < 3; round++ {
		var diff diffAnswer
		if err := client.call(context.Background(), "click", map[string]any{"mark": 1}, &diff); err != nil {
			t.Fatalf("round %d failed: %v", round, err)
		}
	}

	// The worker saw what went over the wire, which is the only thing the
	// protocol promises: three requests, numbered one, two and three in order,
	// so that no answer can ever be taken for another request's.
	if numbers := worker.identifiersAsked(); !slices.Equal(numbers, []int64{1, 2, 3}) {
		t.Errorf("the worker was asked under the request numbers %v, want 1, 2 and 3 in order", numbers)
	}
}

func TestTheClientTurnsTheWorkersRefusalIntoAnErrorThatCarriesItsCode(t *testing.T) {
	worker := newScriptedWorker()
	worker.refuse("click", &workerFailure{
		Code:    codeNoSuchMark,
		Message: "there is no control numbered 9 on the screen",
		Data:    json.RawMessage(`{"marks":[]}`),
	})
	client := newClient(worker.start())

	err := client.call(context.Background(), "click", map[string]any{"mark": 9}, &diffAnswer{})

	var failure *workerFailure
	if !errors.As(err, &failure) {
		t.Fatalf("the error is %v, want one carrying the protocol's code", err)
	}
	if failure.Code != codeNoSuchMark {
		t.Errorf("the code is %d, want %d", failure.Code, codeNoSuchMark)
	}
	if !strings.Contains(failure.Error(), "numbered 9") {
		t.Errorf("the message is %q, want it to name the control", failure.Error())
	}
}

func TestTheThreeCodesThatMeanTheWorkerIsBrokenStartItAgainAndTheRestDoNot(t *testing.T) {
	for _, code := range []int{codeParseError, codeInvalidRequest, codeDriverUnavailable} {
		desk := newDesk(t)
		desk.launched(t)
		first := desk.latestWorker(t)
		first.refuse("click", &workerFailure{Code: code, Message: "the worker is broken"})

		_ = desk.desktop.Click(context.Background(), 1, "the dialog closes")

		if !first.wasStopped() {
			t.Errorf("the code %d left the worker running, and the protocol's table says a worker that answers with it is started again", code)
		}
	}
	for _, code := range []int{codeNoSuchMark, codeBadParameters, codeNoSuchMethod, codeUnreadableWindow, codeNoApplicationOpen, codeLaunchFailed} {
		desk := newDesk(t)
		desk.launched(t)
		first := desk.latestWorker(t)
		first.refuse("click", &workerFailure{Code: code, Message: "the model asked for something that is not there"})

		err := desk.desktop.Click(context.Background(), 1, "the dialog closes")

		if first.wasStopped() {
			t.Errorf("the code %d stopped the worker, and the protocol's table says the model simply hears about it", code)
		}
		if err == nil || !strings.Contains(err.Error(), "not there") {
			t.Errorf("the code %d gave the model %v, want the worker's own message", code, err)
		}
	}
}

func TestTheClientRefusesAnAnswerThatIsNotTheOneItAskedFor(t *testing.T) {
	worker := newScriptedWorker()
	worker.sendRaw("health", `{"jsonrpc":"2.0","id":77,"result":{"healthy":true}}`)
	client := newClient(worker.start())

	err := client.call(context.Background(), "health", map[string]any{}, &healthAnswer{})

	if err == nil || !strings.Contains(err.Error(), "77") {
		t.Fatalf("the error is %v, want one naming the answer that did not match the request", err)
	}
}

func TestTheClientRefusesALineThatIsNotJSON(t *testing.T) {
	worker := newScriptedWorker()
	worker.sendRaw("health", "this is not JSON at all")
	client := newClient(worker.start())

	err := client.call(context.Background(), "health", map[string]any{}, &healthAnswer{})

	if err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("the error is %v, want one saying the line could not be read", err)
	}
}

func TestTheClientRefusesAnAnswerThatIsNeitherAResultNorAnError(t *testing.T) {
	worker := newScriptedWorker()
	worker.sendRaw("health", `{"jsonrpc":"2.0","id":1}`)
	client := newClient(worker.start())

	err := client.call(context.Background(), "health", map[string]any{}, &healthAnswer{})

	if err == nil {
		t.Fatal("an answer with neither a result nor an error was accepted, and it must not be")
	}
}

func TestTheClientGivesUpWhenTheWorkerNeverAnswers(t *testing.T) {
	worker := newScriptedWorker()
	worker.staySilent("health")
	client := newClient(worker.start())
	ctx, giveUp := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer giveUp()

	err := client.call(ctx, "health", map[string]any{}, &healthAnswer{})

	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("the error is %v, want one saying the worker did not answer in time", err)
	}
}

func TestTheClientRefusesALineLongerThanTheCap(t *testing.T) {
	// Eight megabytes and one character, written out rather than measured
	// against the cap, so that raising the cap does not carry the test with it.
	const longerThanTheCap = 8<<20 + 1
	worker := newScriptedWorker()
	worker.sendRaw("health", `{"jsonrpc":"2.0","id":1,"result":{"detail":"`+strings.Repeat("x", longerThanTheCap)+`"}}`)
	client := newClient(worker.start())

	err := client.call(context.Background(), "health", map[string]any{}, &healthAnswer{})

	if err == nil || !strings.Contains(err.Error(), "longer than") {
		t.Fatalf("the error is %v, want one saying the line was longer than the cap", err)
	}
}

func TestTheClientReportsAWorkerThatDiedRatherThanHanging(t *testing.T) {
	worker := newScriptedWorker()
	connection := worker.start()
	client := newClient(connection)
	if err := connection.Stop(); err != nil {
		t.Fatalf("stopping the scripted worker failed: %v", err)
	}

	err := client.call(context.Background(), "health", map[string]any{}, &healthAnswer{})

	if err == nil {
		t.Fatal("a call to a worker that is gone was reported as a success, and it must be an error")
	}
}

func TestTheClientGivesUpOnAWorkerThatSaysNothingWithinTheMethodsDeadline(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	desk.latestWorker(t).staySilent("clipboardGet")
	ctx, giveUp := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer giveUp()

	_, err := desk.desktop.Clipboard(ctx)

	if err == nil {
		t.Fatal("a clipboard read from a worker that never answered was reported as a success")
	}
}
