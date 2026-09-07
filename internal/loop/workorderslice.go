package loop

import (
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/workorder"
)

// The two lines the slice of a work order is written under in a job task's
// front: the sections the task named, in full, and the rest by heading.
const (
	// TheDetailsHeading opens the sections the task's line names.
	TheDetailsHeading = "Details for this task:"
	// TheOtherSectionsLine lists the sections the task did not name, by
	// heading only, and says how to read one.
	TheOtherSectionsLine = "Other sections of the ask, read one with `read ask <heading>`:"
)

// theJobSummaryOf prints the job above a task the way the model reads it. A
// job made from a work order shows the goal and the where in place of the
// whole ask, and under the summary the task's own slice of the Details: the
// sections its line names in full and the rest by heading, so that a task on
// the dragon reads the dragon and not the yeti. Any other job prints as it
// always did, and the job's ask is kept whole for `read ask`.
func (running *run) theJobSummaryOf(held contract.Record) string {
	order := workorder.Parse(held.Goal.Ask)
	if !order.IsWorkOrder {
		return string(record.Print(held))
	}
	running.jobAsk = held.Goal.Ask
	view := held
	view.Goal.Ask = order.Goal
	if order.Where != "" {
		view.Goal.Ask += "\nWhere: " + order.Where
	}
	return string(record.Print(view)) + "\n" + theSliceOf(order, running.task.Message.Text)
}

// theSliceOf writes the sections a task names in full and the others by
// heading. A task naming none is shown every heading by line.
func theSliceOf(order workorder.WorkOrder, taskLine string) string {
	named := workorder.DetailsNamedBy(taskLine)
	var out strings.Builder
	var others []string
	shown := 0
	for _, section := range order.Sections {
		if !namesHeading(named, section.Heading) {
			others = append(others, section.Heading)
			continue
		}
		if shown == 0 {
			out.WriteString(TheDetailsHeading + "\n")
		}
		shown++
		out.WriteString(strings.Repeat("#", section.Level) + " " + section.Heading + "\n")
		out.WriteString(strings.TrimSpace(section.Body) + "\n\n")
	}
	if len(others) > 0 {
		out.WriteString(TheOtherSectionsLine + " " + strings.Join(others, ", ") + "\n")
	}
	return out.String()
}

// namesHeading says whether the task's names include the heading, without
// regard to case or surrounding spaces.
func namesHeading(named []string, heading string) bool {
	for _, name := range named {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(heading)) {
			return true
		}
	}
	return false
}
