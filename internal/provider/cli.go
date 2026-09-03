package provider

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The bounds and the fixed wait a run of a vendor program works inside.
const (
	// fixedProgramRetryAfter is how long to wait when a program says it has hit
	// a usage limit, which it reports in words rather than in a header.
	fixedProgramRetryAfter = 60 * time.Second
	// maxSystemPromptBytes caps the system prompt written for one run. Both
	// programs are handed it in a file rather than on the command line, so the
	// kernel's limit of 128 kilobytes on one argument is not what this bound is
	// about; half a megabyte is more system prompt than the largest window this
	// harness talks to can hold, so a bigger one is a fault further up.
	maxSystemPromptBytes = 512 << 10
	// maxProgramErrorBytes caps how much of a program's complaint is kept.
	maxProgramErrorBytes = 8 << 10
	// killGracePeriod is how long the run is given to finish after its process
	// group has been killed.
	killGracePeriod = 5 * time.Second
)

// programRateLimitPhrases are the ways the two programs say the subscription has
// run out for now.
var programRateLimitPhrases = []string{"usage limit", "rate limit", "rate_limit", "too many requests", "quota"}

// commandLineModel is one model reached by running the vendor's own program on
// the user's subscription. The program has no tool interface, so the tools are
// written into the prompt in the one text form and internal/repair reads the
// calls back out of the reply.
type commandLineModel struct {
	alias   contract.ModelAlias
	options Options
	path    string
}

// newCommandLineModel returns the model a command-line alias names, after
// finding the program on the path so that a missing one is reported at
// startup rather than in the middle of a task.
func newCommandLineModel(alias contract.ModelAlias, options Options) (*commandLineModel, error) {
	if alias.Program != contract.ClaudeProgram && alias.Program != contract.CodexProgram {
		return nil, fmt.Errorf("the model alias %q names the program %q, and the two this harness can drive are %q and %q",
			alias.Name, alias.Program, contract.ClaudeProgram, contract.CodexProgram)
	}
	path, err := exec.LookPath(alias.Program)
	if err != nil {
		return nil, fmt.Errorf("the program %q that the model alias %q needs is not on the path, so install it and sign in: %w",
			alias.Program, alias.Name, err)
	}
	return &commandLineModel{alias: alias, options: options, path: path}, nil
}

// Name is the alias the user gave this model.
func (model *commandLineModel) Name() string { return model.alias.Name }

// ContextLength is how many tokens the model holds on one call.
func (model *commandLineModel) ContextLength() int { return model.alias.ContextLength }

// Send runs the program once in a folder of its own and turns what it printed
// into a reply.
func (model *commandLineModel) Send(ctx context.Context, request contract.Request,
	onDelta func(delta string)) (contract.Reply, error) {
	systemText := renderSystemText(request)
	if len(systemText) > maxSystemPromptBytes {
		return contract.Reply{}, fmt.Errorf("the system prompt for the model %q is %d bytes and this harness sends at most %d, so shorten the context",
			model.alias.Name, len(systemText), maxSystemPromptBytes)
	}
	level := thinkFor(request, model.alias)
	if err := CheckThink(model.alias, level); err != nil {
		return contract.Reply{}, err
	}
	folder, err := model.scratchFolder()
	if err != nil {
		return contract.Reply{}, err
	}
	defer os.RemoveAll(folder)

	arguments, err := model.argumentsFor(folder, systemText, level)
	if err != nil {
		return contract.Reply{}, err
	}
	found, err := model.run(ctx, folder, arguments, renderTranscript(request.Messages), onDelta)
	if err != nil {
		return contract.Reply{}, err
	}
	usage := found.usage
	usage.CostUSD = found.cost
	model.noteUsage(usage)
	if failure := model.failureIn(found); failure != nil {
		return contract.Reply{}, failure
	}
	return contract.Reply{
		Text:   found.text,
		Finish: contract.FinishEnd,
		Usage:  usage,
		Model:  model.alias.Name,
	}, nil
}

// scratchFolder makes a folder of its own under the home's run folder for one
// run, holding nothing but that run's system prompt, so that no CLAUDE.md, no
// project settings, and no hooks from anywhere else can reach the prompt. The
// whole folder is removed when the call ends.
func (model *commandLineModel) scratchFolder() (string, error) {
	root := filepath.Join(model.options.Home.RunFolder(), "cli")
	if err := os.MkdirAll(root, contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("the folder %s for running %s in could not be made: %w", root, model.alias.Program, err)
	}
	folder, err := os.MkdirTemp(root, model.alias.Program+"-")
	if err != nil {
		return "", fmt.Errorf("an empty folder for running %s in could not be made under %s: %w", model.alias.Program, root, err)
	}
	return folder, nil
}

// run starts the program, reads what it prints as it prints it, and waits for it
// to finish. On the deadline the whole process group is killed by the exact
// process identifier, never by anything that matches a name.
func (model *commandLineModel) run(ctx context.Context, folder string, arguments []string,
	prompt string, onDelta func(delta string)) (programResult, error) {
	ctx, releaseDeadline := withCallDeadline(ctx)
	defer releaseDeadline()

	command := exec.CommandContext(ctx, model.path, arguments...)
	command.Dir = folder
	command.Stdin = strings.NewReader(prompt)
	complaint := &cappedWriter{limit: maxProgramErrorBytes}
	command.Stderr = complaint
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = killGracePeriod

	printed, err := command.StdoutPipe()
	if err != nil {
		return programResult{}, fmt.Errorf("the output of %s could not be read: %w", model.alias.Program, err)
	}
	if err := command.Start(); err != nil {
		return programResult{}, fmt.Errorf("the program %s could not be started: %w", model.alias.Program, err)
	}
	found := model.readOutput(printed, onDelta)
	waited := command.Wait()
	if waited != nil && !found.sawResult {
		return programResult{}, fmt.Errorf("the program %s for the model %q ended with %v and said %q",
			model.alias.Program, model.alias.Name, waited, complaint.text())
	}
	return found, nil
}

// readOutput reads the program's output with the parser its shape needs.
func (model *commandLineModel) readOutput(printed io.Reader, onDelta func(delta string)) programResult {
	if model.alias.Program == contract.CodexProgram {
		return parseCodexOutput(printed, onDelta)
	}
	return parseClaudeOutput(printed, onDelta)
}

// noteUsage writes down what one run read, wrote, and cost. It is the only place
// money is logged, because the two subscription programs are the only providers
// that report any.
func (model *commandLineModel) noteUsage(usage contract.Usage) {
	if usage.CostUSD > 0 {
		model.options.note("the model %q used %d tokens in, %d of them cached, and %d out, and cost %.5f dollars",
			model.alias.Name, usage.InputTokens, usage.CachedInputTokens, usage.OutputTokens, usage.CostUSD)
		return
	}
	model.options.note("the model %q used %d tokens in, %d of them cached, and %d out, and the program reported no cost",
		model.alias.Name, usage.InputTokens, usage.CachedInputTokens, usage.OutputTokens)
}

// failureIn turns a program's own report of trouble into the error the harness
// knows how to act on, and returns nothing when the run went well.
func (model *commandLineModel) failureIn(found programResult) error {
	said := found.message
	if !found.failed {
		if looksLikeOverflow(said) && found.text == "" {
			return fmt.Errorf("the model %q refused the call because the prompt was too long, and it said %q: %w",
				model.alias.Name, said, contract.ErrContextOverflow)
		}
		return nil
	}
	switch {
	case looksLikeOverflow(said):
		return fmt.Errorf("the model %q refused the call because the prompt was too long, and it said %q: %w",
			model.alias.Name, said, contract.ErrContextOverflow)
	case saysItIsOutOfAllowance(said):
		return fmt.Errorf("the subscription behind the model %q has run out for now, and it said %q: %w",
			model.alias.Name, said, contract.RateLimitedError{RetryAfter: fixedProgramRetryAfter})
	default:
		return providerError{modelName: model.alias.Name, message: said, retryable: true}
	}
}

// saysItIsOutOfAllowance says whether a program's complaint is about the
// subscription's allowance rather than about the request.
func saysItIsOutOfAllowance(said string) bool {
	lowered := strings.ToLower(said)
	for _, phrase := range programRateLimitPhrases {
		if strings.Contains(lowered, phrase) {
			return true
		}
	}
	return false
}

// cappedWriter keeps the first part of what is written to it and throws the rest
// away, so that a program that complains for ever cannot fill memory.
type cappedWriter struct {
	limit int
	kept  []byte
}

// Write keeps what fits and reports the whole length, so that the program is
// never told its output was refused.
func (writer *cappedWriter) Write(written []byte) (int, error) {
	if room := writer.limit - len(writer.kept); room > 0 {
		writer.kept = append(writer.kept, written[:min(room, len(written))]...)
	}
	return len(written), nil
}

// text is what was kept.
func (writer *cappedWriter) text() string {
	return strings.TrimSpace(string(writer.kept))
}
