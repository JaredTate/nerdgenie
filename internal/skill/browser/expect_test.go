package browser_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill/browser"
)

// thePageUnderTest is the page every judgement in this file is made against.
func thePageUnderTest() contract.Snapshot {
	return contract.Snapshot{
		URL: "https://fixture.test/report", Title: "Report a problem", TabID: "t1",
		Elements: []contract.Element{
			{Ref: "e1", Role: "textbox", Name: "Your name"},
			{Ref: "e2", Role: "button", Name: "File the report"},
		},
	}
}

func TestAnExpectedStateIsMetWhenOneOfItsOwnWordsIsOnThePage(t *testing.T) {
	met := []string{
		"the report form is on the screen",
		"a button that files the report",
		"the page is titled Report a problem",
		"the address is fixture.test/report",
		"THE REPORT IS THERE",
	}
	for _, state := range met {
		if !browser.StateMet(state, thePageUnderTest()) {
			t.Errorf("the state %q is not met, and the page holds one of its words", state)
		}
	}
}

func TestAnExpectedStateIsNotMetWhenNoneOfItsWordsIsOnThePage(t *testing.T) {
	unmet := []string{
		"the invoice has been paid",
		"a video is playing",
	}
	for _, state := range unmet {
		if browser.StateMet(state, thePageUnderTest()) {
			t.Errorf("the state %q is met, and none of its words is on the page", state)
		}
	}
}

func TestAStateBuiltOnlyOfShortAndCommonWordsIsMetByAnything(t *testing.T) {
	for _, state := range []string{"", "it is a b c", "there should be some of them"} {
		if !browser.StateMet(state, thePageUnderTest()) {
			t.Errorf("the state %q is not met, and it holds nothing to look for", state)
		}
	}
}

func TestTheWordsOfAStateLeaveOutTheOnesThatSayNothing(t *testing.T) {
	words := browser.StateWords("There should be a Button that files the REPORT")
	wanted := []string{"button", "files", "report"}
	if len(words) != len(wanted) {
		t.Fatalf("the words are %v, want %v", words, wanted)
	}
	for at, word := range wanted {
		if words[at] != word {
			t.Fatalf("the words are %v, want %v", words, wanted)
		}
	}
}

func TestWhatIsShownNamesThePageTheWallAndTheDialog(t *testing.T) {
	page := thePageUnderTest()
	page.Wall = &contract.Wall{Kind: contract.WallCaptcha, Detail: "a checkbox named I am not a robot"}
	page.Dialog = &contract.Dialog{Kind: "confirm", Message: "Leave this page?"}

	said := browser.WhatIsShown(page)
	for _, wanted := range []string{"Report a problem", "captcha", "Leave this page?", `button "File the report"`} {
		if !strings.Contains(said, wanted) {
			t.Errorf("what was seen is %q, and it should mention %q", said, wanted)
		}
	}
}

func TestWhatIsShownIsCutOffAtTheCap(t *testing.T) {
	page := thePageUnderTest()
	for number := range 500 {
		page.Elements = append(page.Elements, contract.Element{Role: "link", Name: strings.Repeat("long", number%7+1)})
	}
	said := browser.WhatIsShown(page)
	if len([]rune(said)) <= browser.MaxSeenRunes {
		t.Fatalf("a page of five hundred elements was said in %d characters, so it was not cut off", len([]rune(said)))
	}
	if !strings.HasSuffix(said, "(cut short before the end)") {
		t.Errorf("what was seen was cut off without saying so: %q", said[len(said)-40:])
	}
}
