// The shape of the questions follows Hermes' setup program at
// ~/Code/hermes-agent/hermes_cli/setup.py: a line question with the default in
// brackets, a yes-or-no question, and a numbered menu, each of which takes the
// default when the user simply presses Enter. Hermes asks dozens of them across
// several screens; this asks at most six and nothing else, because a setup a
// person has to read is a setup they will get wrong.

package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// AnswerWait is how long one question waits for an answer before giving up. A
// person who has walked away is better told to run "nerdgenie init" again than left
// with half a home folder.
const AnswerWait = 2 * time.Minute

// pickUpThere is the ending every giving-up message shares. By the time a
// question is asked, the home folder, the folders inside it, the persona files,
// and the work folder are all there, so saying that nothing was set up would be
// untrue: what is missing is config.toml, and a second run writes it.
const pickUpThere = "; the home folder was made, but no configuration was written, so run nerdgenie init again and it picks up there"

// The bounds on the conversation, so that a pipe full of rubbish cannot make
// the setup read for ever.
const (
	// maxConversationBytes is the most that is ever read from the input.
	maxConversationBytes = 64 << 10
	// maxTriesPerQuestion is how many times one question is asked again after an
	// answer that will not do.
	maxTriesPerQuestion = 3
)

// asker puts the questions "nerdgenie init" asks and reads the answers back. When
// there is nobody to ask, because every answer came in as a flag or there is no
// input at all, it says so and the caller takes the default.
type asker struct {
	reader *bufio.Reader
	output io.Writer
	silent bool
}

// newAsker builds the asker. A run with no input, or one told to take every
// default, asks nothing at all.
func newAsker(input io.Reader, output io.Writer, silent bool) *asker {
	ask := &asker{output: output, silent: silent || input == nil}
	if input != nil {
		ask.reader = bufio.NewReader(io.LimitReader(input, maxConversationBytes))
	}
	return ask
}

// canAsk says whether there is anybody to answer a question.
func (ask *asker) canAsk() bool { return !ask.silent && ask.reader != nil }

// line asks a question with a default in brackets and reads one line back. An
// empty answer is the default.
func (ask *asker) line(ctx context.Context, question string, fallback string) (string, error) {
	fmt.Fprintf(ask.output, "\n%s\n[%s] ", question, fallback)
	answer, err := ask.read(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(answer) == "" {
		return fallback, nil
	}
	return strings.TrimSpace(answer), nil
}

// yesOrNo asks a question that has two answers.
func (ask *asker) yesOrNo(ctx context.Context, question string, fallback bool) (bool, error) {
	shown := "yes"
	if !fallback {
		shown = "no"
	}
	for tries := 0; tries < maxTriesPerQuestion; tries++ {
		answer, err := ask.line(ctx, question, shown)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(ask.output, "answer yes or no.")
	}
	return false, fmt.Errorf("the question %q was not answered yes or no in %d tries%s", question, maxTriesPerQuestion, pickUpThere)
}

// choice prints a numbered menu and reads back the number of one of its lines.
func (ask *asker) choice(ctx context.Context, question string, choices []string, fallback int) (int, error) {
	fmt.Fprintf(ask.output, "\n%s\n", question)
	for at, choice := range choices {
		fmt.Fprintf(ask.output, "  %d. %s\n", at+1, choice)
	}

	for tries := 0; tries < maxTriesPerQuestion; tries++ {
		answer, err := ask.line(ctx, "Type the number of the one you want.", strconv.Itoa(fallback+1))
		if err != nil {
			return 0, err
		}
		picked, err := strconv.Atoi(strings.TrimSpace(answer))
		if err == nil && picked >= 1 && picked <= len(choices) {
			return picked - 1, nil
		}
		fmt.Fprintf(ask.output, "type a number from 1 to %d.\n", len(choices))
	}
	return 0, fmt.Errorf("no number from 1 to %d was typed in %d tries%s", len(choices), maxTriesPerQuestion, pickUpThere)
}

// read waits for one line, and gives up when nothing arrives inside the wait, so
// that a setup started by a program that then walks away does not hang for ever.
func (ask *asker) read(ctx context.Context) (string, error) {
	type heard struct {
		line string
		err  error
	}
	answers := make(chan heard, 1)
	go func() {
		line, err := ask.reader.ReadString('\n')
		answers <- heard{line: line, err: err}
	}()

	waiting, stop := context.WithTimeout(ctx, AnswerWait)
	defer stop()
	select {
	case answer := <-answers:
		return readOneLine(answer.line, answer.err)
	case <-waiting.Done():
		return "", fmt.Errorf("no answer came in %s%s", AnswerWait, pickUpThere)
	}
}

// readOneLine turns what the reader handed back into an answer. Text with no
// newline after it is still an answer, because the last line of a script often
// has none; nothing at all means the answers ran out.
func readOneLine(line string, err error) (string, error) {
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("the answer could not be read: %w%s", err, pickUpThere)
	}
	if line == "" {
		return "", errors.New("the answers ran out before every question was asked" + pickUpThere +
			", or give nerdgenie init every answer as a flag instead")
	}
	return strings.TrimRight(line, "\r\n"), nil
}
