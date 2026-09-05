package tui

import (
	"strconv"
	"strings"
)

const (
	// maxSituationLines is how many facts of the record's situation the
	// panel draws, so that a situation with a line for every file is not the
	// whole panel.
	maxSituationLines = 8
	// failureLines is how many lines the newest failure is drawn on before
	// the rest is cut, so that a long cause is not the whole panel.
	failureLines = 2
	// testsLabel is the label of the situation's line about the tests, which
	// is the one line coloured by what it says.
	testsLabel = "tests"
)

// modelPanelLines are the model in use, how full its context is, how warm
// its cache is, and what this session has cost so far, kept quiet: one line
// each, with no box around them. The context is the header's meter with the
// numbers beside it, and the cache share is in the header's own colours.
func (screen *Screen) modelPanelLines() []row {
	width := screen.panelTextWidth()
	lines := appendPanelWords(nil, styleDim, screen.modelAlias, width)
	if measure := screen.contextMeterRow(); measure.width > 0 {
		lines = append(lines, measure)
	}
	if cache := screen.cachePart(); cache.text != "" {
		lines = appendPanelWords(lines, cache.style, cache.text, width)
	}
	return appendPanelWords(lines, styleDim, screen.costWords(), width)
}

// contextMeterRow is the context meter with the numbers beside it, such as
// "▰▱▱▱▱▱▱▱▱▱ 12.4k/262k 5%", cut to the panel, or an empty row when the
// program has not sent both numbers.
func (screen *Screen) contextMeterRow() row {
	share := contextShare(screen.contextTokens, screen.contextWindow)
	measure := row{}
	if share < 0 {
		return measure
	}
	for _, piece := range meterSpans(share, contextMeterCells, meterStyle(share)) {
		measure.addSpan(piece)
	}
	measure.add(styleDim, " "+tokenWords(screen.contextTokens)+"/"+tokenWords(screen.contextWindow))
	measure.add(meterStyle(share), " "+strconv.Itoa(share)+"%")
	return cutRowWithEllipsis(measure, screen.panelTextWidth())
}

// nowPanelLines say what is happening: the state in plain words, the call in
// flight when the last tool line has no result yet, and how long the model
// call has run and how many tokens it has written, or the one word idle.
// Nothing is said before the program has reported itself, because a screen
// that has not heard from the program has nothing to say about now.
func (screen *Screen) nowPanelLines() []row {
	if screen.state == stateConnecting {
		return nil
	}
	width := screen.panelTextWidth()
	lines := appendPanelWords(nil, screen.stateStyle(), screen.stateWords(), width)
	if call := screen.callInFlight(); call != "" {
		line := row{}
		line.addSpan(markRunning.span())
		line.add(styleNormal, cutWithEllipsis(call, width-line.width))
		lines = append(lines, line)
	}
	return appendPanelWords(lines, styleDim, screen.callWords(), width)
}

// callInFlight is the tool and its argument from the last tool line the
// program sent, while that line has no result yet, and nothing once it has.
func (screen *Screen) callInFlight() string {
	read := readPill(screen.lastTool)
	if screen.lastTool == "" || read.done {
		return ""
	}
	return strings.TrimSpace(read.tool + " " + read.argument)
}

// statePanelLines are the record's situation, one line per fact: the label
// before the colon dim, the fact after it plain, and the tests line green
// when it says all and red when it says failing. A fact wider than the panel
// is cut with an ellipsis rather than wrapped, so that the group stays one
// row per fact.
func (screen *Screen) statePanelLines() []row {
	width := screen.panelTextWidth()
	lines := []row{}
	for _, fact := range strings.Split(screen.situation, "\n") {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if len(lines) == maxSituationLines {
			break
		}
		lines = append(lines, cutRowWithEllipsis(factRow(fact), width))
	}
	return lines
}

// factRow draws one fact of the situation: its label dim and the rest in the
// colour the fact earns, or the whole line plain when it has no label.
func factRow(fact string) row {
	line := row{}
	label, rest, labelled := strings.Cut(fact, ":")
	if !labelled {
		line.add(styleNormal, fact)
		return line
	}
	line.add(styleDim, label)
	line.add(factStyle(label, rest), ":"+rest)
	return line
}

// factStyle is the colour of a fact's words: red when the tests line says
// failing, green when it says all, and plain otherwise.
func factStyle(label string, rest string) style {
	if strings.TrimSpace(label) != testsLabel {
		return styleNormal
	}
	switch {
	case strings.Contains(rest, "failing"):
		return styleBad
	case strings.Contains(rest, "all"):
		return styleDone
	default:
		return styleNormal
	}
}

// failuresPanelLines are the record's failures: how many there are, in red,
// and the newest one wrapped to failureLines rows with the rest cut, because
// the newest failure is the one the work is answering now.
func (screen *Screen) failuresPanelLines() []row {
	listed := []string{}
	for _, failure := range strings.Split(screen.failures, "\n") {
		if failure = strings.TrimSpace(failure); failure != "" {
			listed = append(listed, failure)
		}
	}
	if len(listed) == 0 {
		return nil
	}
	width := screen.panelTextWidth()
	count := strconv.Itoa(len(listed)) + " failures"
	if len(listed) == 1 {
		count = "1 failure"
	}
	lines := appendPanelWords(nil, styleBad, count, width)
	wrapped := wrapText(listed[len(listed)-1], width)
	for at, text := range wrapped {
		if at == failureLines-1 && len(wrapped) > failureLines {
			text = cutWithEllipsis(strings.Join(wrapped[at:], " "), width)
		}
		lines = appendPanelWords(lines, styleNormal, text, width)
		if at == failureLines-1 {
			break
		}
	}
	return lines
}

// roundPanelLines say which round the task is on and how long it has run,
// such as "round 27 · 12m in", the round alone when the program has not
// said when the task began, and nothing when it has not said the round.
func (screen *Screen) roundPanelLines() []row {
	if screen.round == "" || screen.round == "0" {
		return nil
	}
	words := "round " + screen.round
	if elapsed := screen.taskElapsedWords(); elapsed != "" {
		words += " · " + elapsed + " in"
	}
	return appendPanelWords(nil, styleDim, words, screen.panelTextWidth())
}

// jobWords says how many jobs are waiting in the words a person would use. A
// count that is empty or zero is nothing at all, because "0 jobs" is noise the
// program did not mean to say.
func jobWords(count string) string {
	switch count {
	case "", "0":
		return ""
	case "1":
		return "1 job waiting"
	default:
		return count + " jobs waiting"
	}
}

// waitingPanelLines is how many jobs are waiting, dim, while no job is on the
// checklist, or nothing at all when the program has not said.
func (screen *Screen) waitingPanelLines() []row {
	if screen.job != "" {
		return nil
	}
	return appendPanelWords(nil, styleDim, jobWords(screen.jobs), screen.panelTextWidth())
}
