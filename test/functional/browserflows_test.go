package functional

// The five browser flows brief 5.5 calls for, and what is left of each one.
//
// Every flow drives the whole program through the same socket the terminal uses,
// with the fakes from internal/testkit for everything but the browser. None of
// them can be written yet: they all go through internal/browser, which is brief
// 5.2 and has not merged. The two halves that need no Go side are written and
// passing: the fixture site itself is proved through plain HTTP in site_test.go,
// and the real worker meeting the fixture login wall and filling it is proved
// against a real Chrome in browserflows_integration_test.go.
//
// These five stay here, named for what they will prove and skipped for one
// stated reason, so that the suite says out loud what is still missing rather
// than saying nothing at all.

import "testing"

// waitsForTheGoSideOfTheBrowser is the one reason all five flows are skipped.
// When brief 5.2 merges, each of them is written and the skip goes.
const waitsForTheGoSideOfTheBrowser = "waits for internal/browser (brief 5.2)"

// TestLoggingInToTheFixtureSiteThroughTheVault will put a fixture entry with a
// TOTP secret in the vault, ask the agent to sign in to the fixture site, and
// prove that the login tool found the entry, checked the page was on one of its
// domains, filled the form with a fresh code, and landed on the compose page
// without any of the three values ever reaching the model.
func TestLoggingInToTheFixtureSiteThroughTheVault(t *testing.T) {
	t.Skip(waitsForTheGoSideOfTheBrowser)
}

// TestPostingWithAPreviewTheUserApproves will ask the agent to post a message on
// the fixture site, prove that the user saw a preview of exactly what would be
// posted on the fake channel, and prove that the post appeared on the compose
// page only after the user said yes.
func TestPostingWithAPreviewTheUserApproves(t *testing.T) {
	t.Skip(waitsForTheGoSideOfTheBrowser)
}

// TestHittingTheCaptchaPageHandsTheBrowserToTheUser will send the agent to the
// fixture captcha page and prove that it stopped, brought the window forward,
// sent a screenshot and a numbered list through the fake channel, and carried on
// only when the user replied "done".
func TestHittingTheCaptchaPageHandsTheBrowserToTheUser(t *testing.T) {
	t.Skip(waitsForTheGoSideOfTheBrowser)
}

// TestReplayingARecordedSkillOnTheFixtureSite will record a three-step flow on
// the fixture site, replay it, and prove the fake model was never called. It
// needs internal/skill/browser from brief 5.3 as well.
func TestReplayingARecordedSkillOnTheFixtureSite(t *testing.T) {
	t.Skip(waitsForTheGoSideOfTheBrowser)
}

// TestTheQualitySkillReportsOnTheFixtureSite will walk the fixture site's form
// with the quality skill and prove it produced a report naming each step, its
// labelled screenshot, and how the page compared with the state expected in
// plain words. It needs skills/qa from brief 5.3 as well.
func TestTheQualitySkillReportsOnTheFixtureSite(t *testing.T) {
	t.Skip(waitsForTheGoSideOfTheBrowser)
}
