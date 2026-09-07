package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
const TheFifthQuestion = "Which section of ARCHITECTURE.md does this task change, and what should that section say now? Put the heading on the first line and the paragraph under it, or say none."

// MaxSectionWords is the most words a section body the review writes may hold.
// A section is what the next task reads to find its bearings, and two hundred
// words is one round of reading.
const MaxSectionWords = 200

// theTitleOfANewPage opens a page the review starts in a folder that had none.
const theTitleOfANewPage = "# Architecture\n"

// writeTheArchitectureSection asks the fifth question and puts the answer on
// the page. Nothing here fails the task: a page that cannot be written costs
// the page and nothing more.
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
	if page == "" {
		page = theTitleOfANewPage
	}
	answer := running.theLoop.askWithTheToolsOff(ctx, string(record.Print(running.keeper.Record())), TheFifthQuestion)
	heading, body, found := readTheSectionAnswer(answer)
	if !found {
		return
	}
	body, cut := cutAtWords(body, MaxSectionWords)
	dated := fmt.Sprintf("(updated by task %s, %s)", running.keeper.ID(), running.theLoop.options.Clock.Now().UTC().Format("2006-01-02"))
	if cut {
		dated = fmt.Sprintf("(updated by task %s, %s; cut at two hundred words)", running.keeper.ID(), running.theLoop.options.Clock.Now().UTC().Format("2006-01-02"))
	}
	written, _ := markdown.ReplaceSection(page, heading, body+"\n\n"+dated+"\n")
	_ = os.WriteFile(path, []byte(written), 0o644)
}

// readTheSectionAnswer reads the heading off the answer's first line and the
// body off the rest. "None", an empty answer, and an answer with no heading
// line are no section.
func readTheSectionAnswer(answer string) (string, string, bool) {
	lines := strings.Split(strings.TrimSpace(answer), "\n")
	first := strings.TrimSpace(lines[0])
	heading := strings.TrimSpace(strings.TrimLeft(first, "#"))
	if heading == "" || strings.EqualFold(strings.Trim(heading, ". "), "none") {
		return "", "", false
	}
	if !strings.HasPrefix(first, "#") || len(strings.Fields(heading)) > 8 {
		return "", "", false
	}
	body := strings.TrimSpace(strings.Join(lines[1:], "\n"))
	if body == "" {
		return "", "", false
	}
	return heading, body, true
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
