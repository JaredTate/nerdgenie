package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aTinyPicture is a one-pixel PNG, as base64.
const aTinyPicture = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// picturingTool is a tool that hands back a picture with its words.
type picturingTool struct{ name string }

func (tool *picturingTool) Spec() contract.ToolSpec {
	return contract.ToolSpec{Name: tool.name, Description: "Takes a picture.", Fields: []contract.ToolField{{Name: "intent", Type: "string", Required: true}}}
}

func (tool *picturingTool) Run(_ context.Context, input json.RawMessage) (contract.ToolOutput, error) {
	return contract.ToolOutput{Text: "the page as a picture, 1024 by 768: " + string(input), Picture: aTinyPicture}, nil
}

// aLookingScript takes the picture this many times and then answers.
func aLookingScript(times int) []testkit.Step {
	steps := []testkit.Step{}
	for at := 1; at <= times; at++ {
		steps = append(steps, callStep("I will look at the page.", callFor(fmt.Sprintf("p%d", at), "snap", fmt.Sprintf(`{"intent":"look %d"}`, at))))
	}
	return append(steps, answerStep("The page looks right. What changed: nothing. What I checked: the page. What is left: nothing."))
}

// TestAPictureRidesWithItsResultWhenTheModelCanSee is the eyes turned on: a
// tool that hands back a picture has it sent to a model that can see.
func TestAPictureRidesWithItsResultWhenTheModelCanSee(t *testing.T) {
	built := newHarnessThatSees(t, aLookingScript(1), &picturingTool{name: "snap"})

	built.ask(t, "look at the page")

	requests := built.model.Requests()
	last := requests[len(requests)-1]
	if picturesIn(last) != 1 {
		t.Errorf("the model was sent %d pictures, want the one the tool took", picturesIn(last))
	}
	if strings.Contains(wholeRequestText(last), loop.ThePictureIsNotShown) {
		t.Error("the model that can see was told the picture is not shown to it")
	}
}

// TestAPictureIsSaidToBeUnseenWhenTheModelCannot keeps the old truth for a
// model with no eyes: the picture is dropped and the result says so, which is
// what the fifth game build's play-test needed to stop asking for screenshots.
func TestAPictureIsSaidToBeUnseenWhenTheModelCannot(t *testing.T) {
	built := newHarness(t, aLookingScript(1), &picturingTool{name: "snap"})

	built.ask(t, "look at the page")

	requests := built.model.Requests()
	last := requests[len(requests)-1]
	if picturesIn(last) != 0 {
		t.Errorf("the model that cannot see was sent %d pictures", picturesIn(last))
	}
	if !strings.Contains(wholeRequestText(last), loop.ThePictureIsNotShown) {
		t.Error("the model that cannot see was not told the picture is not shown to it")
	}
}

// TestOnlyTheNewestPicturesStayInTheWindow bounds what pictures cost: a
// screenshot is about eight hundred tokens, so only the newest few ride, and
// an older result says its picture is no longer shown and how to see it again.
func TestOnlyTheNewestPicturesStayInTheWindow(t *testing.T) {
	built := newHarnessThatSees(t, aLookingScript(loop.MaxPicturesShown+2), &picturingTool{name: "snap"})

	built.ask(t, "look at the page")

	requests := built.model.Requests()
	last := requests[len(requests)-1]
	if picturesIn(last) != loop.MaxPicturesShown {
		t.Errorf("the model was sent %d pictures on the last call, want the newest %d only", picturesIn(last), loop.MaxPicturesShown)
	}
	if !strings.Contains(wholeRequestText(last), loop.ThePictureIsNoLongerShown) {
		t.Error("the older results do not say their picture is no longer shown")
	}
}

// picturesIn counts the tool results in a request that still carry a picture.
func picturesIn(request contract.Request) int {
	count := 0
	for _, message := range request.Messages {
		for _, result := range message.ToolResults {
			if result.Picture != "" {
				count++
			}
		}
	}
	return count
}
