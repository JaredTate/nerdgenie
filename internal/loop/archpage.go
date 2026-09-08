package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/markdown"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// ArchitectureFile is the page that says how a project is put together, one
// section per part. The review keeps it: at the end of a task it asks which
// section the task changed and what it should say now, so the next task, and
// the next job on the same folder, read a page instead of reading files.
const ArchitectureFile = "ARCHITECTURE.md"

// TheFifthQuestion follows the four of the review when the work folder holds
// an architecture page, or when the task belongs to a job and could start one.
const TheFifthQuestion = "Which section of ARCHITECTURE.md does this task change, and what should that section say now? Put the heading on the first line and the paragraph under it, or say none. " + TheToolsAreOffLine

// TheToolsAreOffLine ends both questions. Run 19's first task answered the
// section question by asking for the read tool, because nothing had told it
// the tools were off, and a page section came out as tool markup.
const TheToolsAreOffLine = "The tools are off: answer from the record above, in words, with no tool call."

// TheFirstSectionQuestion is asked instead when the folder has no page yet:
// the first task of a project is asked to start the page, not which section
// it changed, because "which section changed" invites the answer none.
const TheFirstSectionQuestion = "This project has no ARCHITECTURE.md yet. Start it with the part this task built: put the part's name as a heading on the first line, and under it one paragraph saying what the part is for, what it holds, and which files it lives in. " + TheToolsAreOffLine

// MaxSectionWords is the most words a section body the review writes may hold.
// A section is what the next task reads to find its bearings, and two hundred
// words is one round of reading.
const MaxSectionWords = 200

// MaxHeadingWords is the most words a line may hold and still be read as a
// section's heading. A longer first line is a sentence, and the answer is a
// paragraph that goes under the task's own name.
const MaxHeadingWords = 8

// TheSectionWrittenLine opens the line a task's report ends with when the
// review wrote a section of the page, so the person and the next task know
// which section exists.
const TheSectionWrittenLine = "wrote " + ArchitectureFile + ", section "

// theTitleOfANewPage opens a page the review starts in a folder that had none.
const theTitleOfANewPage = "# Architecture\n"

// writeTheArchitectureSection asks the section question, puts the answer on
// the page, and writes the question, the answer and what came of it into the
// log. Nothing here fails the task: a page that cannot be written costs the
// page and nothing more, and the log says why.
func (running *run) writeTheArchitectureSection(ctx context.Context) {
	folder := running.folder()
	if folder == "" || running.keeper == nil {
		return
	}
	path := filepath.Join(folder, ArchitectureFile)
	held, err := os.ReadFile(path)
	if err != nil && running.task.FromJob == nil {
		return
	}
	page := string(held)
	question := TheFifthQuestion
	if page == "" {
		page = theTitleOfANewPage
		question = TheFirstSectionQuestion
	}
	answer := running.theLoop.askWithTheToolsOff(ctx, string(record.Print(running.keeper.Record())), question)
	outcome := running.putTheAnswerOnThePage(path, page, answer)
	running.theLoop.logTheQuestion(ctx, running.taskID(), "architecture section", question, answer, outcome)
}

// putTheAnswerOnThePage reads the section off the answer and writes it into
// the page, and says what it did in one line: the section written, that there
// was no section, or why the page could not be written.
func (running *run) putTheAnswerOnThePage(path string, page string, answer string) string {
	heading, body, found := readTheSectionAnswer(answer, theFallbackHeading(running.keeper.Record().Goal.Name, running.task.Message.Text))
	if !found {
		return theReasonForNoSection(answer)
	}
	heading = theHeadingOnThePage(page, heading)
	body, cut := cutAtWords(body, MaxSectionWords)
	dated := fmt.Sprintf("(updated by task %s, %s)", running.keeper.ID(), running.theLoop.options.Clock.Now().UTC().Format("2006-01-02"))
	if cut {
		dated = fmt.Sprintf("(updated by task %s, %s; cut at two hundred words)", running.keeper.ID(), running.theLoop.options.Clock.Now().UTC().Format("2006-01-02"))
	}
	written, _ := markdown.ReplaceSection(page, heading, body+"\n\n"+dated+"\n")
	if err := os.WriteFile(path, []byte(written), 0o644); err != nil {
		return "the page could not be written: " + err.Error()
	}
	running.sectionWritten = heading
	return TheSectionWrittenLine + heading
}

// withTheSectionLine adds the line saying which section of the page the task
// wrote to its report, once, so that the person reads it and the job carries
// it to the next task.
func (running *run) withTheSectionLine(report string) string {
	if running.sectionWritten == "" {
		return report
	}
	line := TheSectionWrittenLine + running.sectionWritten
	if strings.Contains(report, line) {
		return report
	}
	return report + "\n" + line
}

// readTheSectionAnswer reads the heading and the body off the answer. The
// heading is the first line when it is a hash heading, or a plain or bold
// line of at most MaxHeadingWords that is not a sentence; a first line that is
// a sentence makes the whole answer the body under the fallback heading, the
// task's own name. "None", an empty answer, a single sentence saying nothing
// changed, a list and a question are no section.
func readTheSectionAnswer(answer string, fallback string) (string, string, bool) {
	lines := strings.Split(strings.TrimSpace(answer), "\n")
	first := strings.TrimSpace(lines[0])
	rest := strings.TrimSpace(strings.Join(lines[1:], "\n"))
	if first == "" || saysThereIsNoSection(first, rest) || looksLikeToolMarkup(answer) {
		return "", "", false
	}
	if strings.HasPrefix(first, "#") {
		heading, isAHeading := headingOn(first)
		if !isAHeading || rest == "" {
			return "", "", false
		}
		return heading, rest, true
	}
	if at := theHashHeadingInside(lines); at > 0 {
		// A sentence or two of preamble, then the section itself: run 20's
		// first task wrote "Here's the first section:" and then the heading.
		heading, _ := headingOn(strings.TrimSpace(lines[at]))
		body := strings.TrimSpace(strings.Join(lines[at+1:], "\n"))
		if body == "" {
			return "", "", false
		}
		return heading, body, true
	}
	if heading, isAHeading := headingOn(first); isAHeading {
		if rest == "" {
			return "", "", false
		}
		return heading, rest, true
	}
	if fallback == "" || opensAsNarration(first) || opensAListOrAQuestion(first) {
		return "", "", false
	}
	return fallback, strings.TrimSpace(answer), true
}

// theHashHeadingInside is the index of the first hash heading after the first
// line, or zero when there is none.
func theHashHeadingInside(lines []string) int {
	for at := 1; at < len(lines); at++ {
		line := strings.TrimSpace(lines[at])
		if strings.HasPrefix(line, "#") {
			if _, isAHeading := headingOn(line); isAHeading {
				return at
			}
		}
	}
	return 0
}

// saysThereIsNoSection says whether the answer declines: it opens with the
// word none, or it is one sentence saying nothing changed.
func saysThereIsNoSection(first string, rest string) bool {
	words := strings.Fields(strings.ToLower(strings.Trim(first, "#*_ ")))
	if len(words) == 0 {
		return true
	}
	if strings.Trim(words[0], ".,:;!") == "none" {
		return true
	}
	return rest == "" && strings.Contains(strings.ToLower(first), "nothing")
}

// headingOn reads a heading off a line: a hash heading of any length up to
// the cap, or a plain or bold line under the cap that does not end the way a
// sentence or a question does.
func headingOn(line string) (string, bool) {
	hashed := strings.HasPrefix(line, "#")
	heading := strings.Trim(strings.TrimLeft(line, "#"), "*_ ")
	heading = strings.TrimSpace(strings.TrimSuffix(heading, ":"))
	if heading == "" || len(strings.Fields(heading)) > MaxHeadingWords {
		return "", false
	}
	if !hashed && (strings.ContainsAny(heading[len(heading)-1:], ".?!;") || opensAListOrAQuestion(line)) {
		return "", false
	}
	return heading, true
}

// theReasonForNoSection is the outcome logged when the answer gave no
// section: the plain "no section" when the model declined, and why when the
// harness refused what it wrote, so the log says which.
func theReasonForNoSection(answer string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(answer), "\n")
	switch {
	case looksLikeToolMarkup(answer):
		return "no section: the answer was tool markup"
	case opensAsNarration(first):
		return "no section: the answer narrated instead of answering"
	}
	return "no section"
}

// theOpeningsOfNarration are how an answer begins when the model is telling
// what it is about to do instead of answering: run 19's first task opened
// with "I'll start by looking at what was built" and then wrote tool markup.
// The answer is still in the log, so the reason is seen.
var theOpeningsOfNarration = []string{"i'll ", "i will ", "i'm going to ", "let me ", "the user wants ", "the user is asking ", "first, i ", "first i "}

// opensAsNarration says whether the first line is the model narrating its
// own next step rather than writing the section.
func opensAsNarration(first string) bool {
	opening := strings.ToLower(strings.Trim(first, "#*_ "))
	for _, sign := range theOpeningsOfNarration {
		if strings.HasPrefix(opening, sign) {
			return true
		}
	}
	return false
}

// opensAListOrAQuestion says whether a line begins a list or ends as a
// question, neither of which names a section.
func opensAListOrAQuestion(line string) bool {
	if strings.HasSuffix(line, "?") || strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	number := strings.TrimLeft(line, "0123456789")
	return number != line && (strings.HasPrefix(number, ".") || strings.HasPrefix(number, ")"))
}

// theFallbackHeading is the name a paragraph with no heading goes under: the
// record's name when the model gave the task one, else the first line of the
// ask. A job task's name is its whole line, "Scaffold: `package.json`, a test
// runner, ...", so the heading is what stands before the colon, with the
// backticks taken off, cut to MaxHeadingWords, with a capital letter.
func theFallbackHeading(name string, ask string) string {
	source := strings.TrimSpace(name)
	if source == "" && !strings.HasPrefix(strings.TrimSpace(ask), TheJobPickUpLine) {
		// A picked-up task's ask is the pick-up line, which names no part:
		// run 23's page got a section headed "The harness stopped this task".
		source, _, _ = strings.Cut(strings.TrimSpace(ask), "\n")
	}
	if before, _, found := strings.Cut(source, ":"); found && strings.TrimSpace(before) != "" {
		source = before
	}
	words := strings.Fields(strings.ReplaceAll(strings.Trim(source, ": "), "`", ""))
	if len(words) == 0 {
		return ""
	}
	if len(words) > MaxHeadingWords {
		words = words[:MaxHeadingWords]
	}
	letters := []rune(strings.Join(words, " "))
	letters[0] = unicode.ToUpper(letters[0])
	return string(letters)
}

// cutAtWords keeps the body under the word cap, cutting at the last sentence
// end inside the cap, and says whether it cut.
func cutAtWords(body string, most int) (string, bool) {
	words := strings.Fields(body)
	if len(words) <= most {
		return body, false
	}
	kept := strings.Join(words[:most], " ")
	if end := strings.LastIndexAny(kept, ".!?"); end > 0 {
		kept = kept[:end+1]
	}
	return kept, true
}

// askWithTheToolsOff makes one call with the tools off over the record and
// returns the reply's text, or nothing when the model could not be asked.
func (theLoop *Loop) askWithTheToolsOff(ctx context.Context, background string, question string) string {
	request, err := theLoop.options.Context.Build(ctx, BuildInput{
		Messages: []contract.Message{
			{Role: contract.RoleUser, Text: background},
			{Role: contract.RoleUser, Text: question},
		},
		ToolsOff:      true,
		ContextLength: theLoop.options.Model.ContextLength(),
	})
	if err != nil {
		return ""
	}
	reply, err := theLoop.options.Model.Send(ctx, request, nil)
	if err != nil {
		return ""
	}
	return reply.Text
}

// MaxLoggedAnswerRunes is the most of a tools-off answer the log keeps. A
// section or a review is a paragraph; anything past this is a model that
// did not answer the question.
const MaxLoggedAnswerRunes = 32 << 10

// theQuestionAsked is the body of a question event: what the harness asked
// with the tools off, why, what the model answered, and what the harness did
// with the answer.
type theQuestionAsked struct {
	// Purpose says what the question was for: "review" or "architecture section".
	Purpose string `json:"purpose"`
	// Question is the question's text, word for word.
	Question string `json:"question"`
	// Answer is the model's reply, word for word, bounded.
	Answer string `json:"answer"`
	// Outcome is what the harness did with the answer, in one line.
	Outcome string `json:"outcome"`
}

// logTheQuestion writes one tools-off question, its answer and what came of
// it into the log under the task or the job, so that a page that was not
// written, or a lesson that was not kept, can be traced to the answer. A log
// that will not take it costs the trace and nothing more: the question is
// bookkeeping, and the work it belongs to is already over.
func (theLoop *Loop) logTheQuestion(ctx context.Context, key string, purpose string, question string, answer string, outcome string) {
	if letters := []rune(answer); len(letters) > MaxLoggedAnswerRunes {
		answer = string(letters[:MaxLoggedAnswerRunes])
	}
	_ = theLoop.logEvent(ctx, key, contract.EventQuestion, theQuestionAsked{Purpose: purpose, Question: question, Answer: answer, Outcome: outcome})
}
