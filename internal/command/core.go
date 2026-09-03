package command

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// Help is the "/help" command: the one listing of everything Coeus answers, in
// the order the commands were registered.
func (commands *Commands) Help() contract.Command {
	return contract.Command{
		Name: "help",
		Help: "Shows this list of commands.",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			return commands.registry.Help(), nil
		},
	}
}

// Status is the "/status" command: the model in use, what the session has cost
// so far, the jobs, what is waiting for an answer, and whether each channel is
// working.
func (commands *Commands) Status() contract.Command {
	return contract.Command{
		Name: "status",
		Help: "Shows the model, the cost so far, the jobs, the answers waiting, and the health of each channel.",
		Run: func(ctx context.Context, _ string, _ contract.CommandContext) (string, error) {
			lines := []string{
				"model: " + commands.modelLine(),
				"cost so far: " + commands.costLine(),
			}
			lines = append(lines, commands.jobLines(ctx)...)
			lines = append(lines, commands.waitingLines(ctx)...)
			lines = append(lines, commands.channelLines(ctx)...)
			return strings.Join(lines, "\n") + "\n", nil
		},
	}
}

// modelLine is the model in use with how hard it is thinking, which is what the
// status prints first. A model left at its provider's own default says nothing
// extra, because that is the setting almost every model is on.
func (commands *Commands) modelLine() string {
	name := commands.modelInUse()
	for _, alias := range commands.deps.Settings.Models {
		if alias.Name == name && alias.Think != contract.ThinkDefault {
			return name + ", thinking at " + string(alias.Think)
		}
	}
	return name
}

// modelInUse is the alias the program is talking to, or the one the
// configuration names when nothing has said otherwise.
func (commands *Commands) modelInUse() string {
	if commands.deps.CurrentModel != nil {
		return commands.deps.CurrentModel()
	}
	if commands.deps.Settings.DefaultModel != "" {
		return commands.deps.Settings.DefaultModel
	}
	return "not chosen yet"
}

// costLine is what the session has spent, written the way the record's header
// writes it.
func (commands *Commands) costLine() string {
	if commands.deps.CostSoFar == nil {
		return "not counted in this build"
	}
	cost := commands.deps.CostSoFar()
	if cost.InputTokens == 0 && cost.OutputTokens == 0 {
		return "nothing yet"
	}
	return fmt.Sprintf("%d tokens in, %d of them cached, %d out",
		cost.InputTokens, cost.CachedInputTokens, cost.OutputTokens)
}

// jobLines are the jobs line and one line per job. A job is the scheduled work
// in Coeus: "/cron" shows only the ones that also carry a clock schedule.
func (commands *Commands) jobLines(ctx context.Context) []string {
	if commands.deps.Jobs == nil {
		return []string{"jobs: no job store in this build"}
	}
	listed, err := commands.deps.Jobs.List(ctx)
	if err != nil {
		return []string{"jobs: could not be listed: " + err.Error()}
	}
	if len(listed) == 0 {
		return []string{"jobs: none"}
	}

	counts := map[contract.JobState]int{}
	lines := make([]string, 1, len(listed)+1)
	for _, job := range listed {
		counts[job.State]++
		lines = append(lines, fmt.Sprintf("  job %s %s, %d of %d tasks done: %s",
			job.ID, job.State, job.TasksDone, job.TasksTotal, job.Title))
	}
	lines[0] = "jobs: " + countedByState(counts)
	return lines
}

// countedByState writes the job counts the way a person would say them, in the
// order running, paused, off, done.
func countedByState(counts map[contract.JobState]int) string {
	said := []string{}
	for _, state := range []contract.JobState{contract.JobRunning, contract.JobPaused, contract.JobOff, contract.JobDone} {
		if counts[state] > 0 {
			said = append(said, fmt.Sprintf("%d %s", counts[state], state))
		}
	}
	return strings.Join(said, ", ")
}

// waitingLines are the previews and questions waiting for an answer, which
// "/approve" and "/deny" answer by the ids printed here.
func (commands *Commands) waitingLines(ctx context.Context) []string {
	if commands.deps.PendingPreviews == nil {
		return []string{"waiting for an answer: nothing asks in this build"}
	}
	waiting, err := commands.deps.PendingPreviews(ctx)
	if err != nil {
		return []string{"waiting for an answer: could not be listed: " + err.Error()}
	}
	if len(waiting) == 0 {
		return []string{"waiting for an answer: nothing"}
	}

	lines := []string{fmt.Sprintf("waiting for an answer: %d", len(waiting))}
	for _, preview := range waiting {
		lines = append(lines, "  "+preview.ID+" "+preview.Title)
	}
	return lines
}

// channelLines say whether each channel is carrying messages, with the reason
// beside any that is not.
func (commands *Commands) channelLines(ctx context.Context) []string {
	if commands.deps.Channels == nil {
		return []string{"channels: none in this build"}
	}
	channels := commands.deps.Channels()
	if len(channels) == 0 {
		return []string{"channels: none"}
	}

	widest := 0
	for _, channel := range channels {
		if len(channel.Name()) > widest {
			widest = len(channel.Name())
		}
	}
	lines := []string{"channels:"}
	for _, channel := range channels {
		health := channel.Health(ctx)
		said := "ok"
		if !health.Healthy {
			said = "not working: " + health.Detail
		}
		lines = append(lines, fmt.Sprintf("  %-*s  %s", widest, channel.Name(), said))
	}
	return lines
}

// Model is the "/model" command: it shows the alias in use with the aliases
// config.toml names, and sets one of them for the rest of this session.
func (commands *Commands) Model() contract.Command {
	return contract.Command{
		Name: "model",
		Help: "Shows the model in use, or sets it to one of the aliases config.toml names.",
		Run: func(_ context.Context, arguments string, _ contract.CommandContext) (string, error) {
			named := aliasNames(commands.deps.Settings)
			wanted := strings.TrimSpace(arguments)
			if wanted == "" {
				return fmt.Sprintf("the model is %s. The aliases config.toml names are %s.",
					commands.modelInUse(), inPlainList(named)), nil
			}
			if !slices.Contains(named, wanted) {
				return fmt.Sprintf("there is no model alias called %q, so use one of %s, or add a [[models]] block to config.toml.",
					wanted, inPlainList(named)), nil
			}
			if commands.deps.SetModel == nil {
				return "", notWiredUp("model", "SetModel")
			}
			if err := commands.deps.SetModel(wanted); err != nil {
				return "", fmt.Errorf("the model could not be set to %q: %w", wanted, err)
			}
			return fmt.Sprintf("the model is now %s, until this session ends or you set it again.", wanted), nil
		},
	}
}

// aliasNames are the model aliases the configuration defines, in the order the
// file writes them.
func aliasNames(settings contract.Config) []string {
	named := make([]string, 0, len(settings.Models))
	for _, alias := range settings.Models {
		named = append(named, alias.Name)
	}
	return named
}

// inPlainList writes a list of words the way a person would say it, with "and"
// before the last one.
func inPlainList(words []string) string {
	switch len(words) {
	case 0:
		return "nothing"
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
	}
}
