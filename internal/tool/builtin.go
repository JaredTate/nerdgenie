package tool

import (
	"context"
	"fmt"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/tool/browseract"
	"github.com/JaredTate/nerdgenie/internal/tool/browserclick"
	"github.com/JaredTate/nerdgenie/internal/tool/browserhandoff"
	"github.com/JaredTate/nerdgenie/internal/tool/browserlogin"
	"github.com/JaredTate/nerdgenie/internal/tool/browseropen"
	"github.com/JaredTate/nerdgenie/internal/tool/browserread"
	"github.com/JaredTate/nerdgenie/internal/tool/browsertype"
	"github.com/JaredTate/nerdgenie/internal/tool/computer"
	"github.com/JaredTate/nerdgenie/internal/tool/edit"
	"github.com/JaredTate/nerdgenie/internal/tool/job"
	"github.com/JaredTate/nerdgenie/internal/tool/memory"
	"github.com/JaredTate/nerdgenie/internal/tool/read"
	"github.com/JaredTate/nerdgenie/internal/tool/search"
	"github.com/JaredTate/nerdgenie/internal/tool/shell"
	"github.com/JaredTate/nerdgenie/internal/tool/skill"
	"github.com/JaredTate/nerdgenie/internal/tool/task"
	"github.com/JaredTate/nerdgenie/internal/tool/web"
	"github.com/JaredTate/nerdgenie/internal/tool/write"
)

// New returns the whole set of tools one turn can see: the eighteen built-in
// ones, in the order design section 7 lists them, and then every executable the
// user dropped into the tools folder.
func New(ctx context.Context, settings Settings) (*Registry, error) {
	registry, err := NewRegistry(settings)
	if err != nil {
		return nil, err
	}
	for _, built := range builtInTools(settings) {
		if err := registry.Add(built); err != nil {
			return nil, fmt.Errorf("cannot register the built-in tool %q: %w", built.Spec().Name, err)
		}
	}
	if err := registry.AddUserTools(ctx); err != nil {
		return nil, err
	}
	return registry, nil
}

// builtInTools builds the eighteen, in the order the design lists them.
func builtInTools(settings Settings) []contract.Tool {
	// The check is held as a plain function rather than as its own named type,
	// so that each tool's own name for the same shape takes it without a cast.
	// It is wrapped so that a path is made whole before it is judged: a short
	// path is taken from the folder the agent works in and ~ is read as the
	// user's home, because every model writes both. Which check it wraps, the
	// sandbox roots or the open one, is the sandbox setting's to say.
	var allowed func(path string) (string, error) = MadeWhole(
		settings.pathCheck(), settings.workingDirectory(), settings.UserHome)
	tools := []contract.Tool{
		read.New(read.Settings{Allowed: allowed, Results: settings.Results, Reports: settings.Reports}),
		write.New(write.Settings{Allowed: allowed, Log: settings.Log, TaskID: settings.TaskID, Clock: settings.Clock}),
		edit.New(edit.Settings{Allowed: allowed, Log: settings.Log, TaskID: settings.TaskID, Clock: settings.Clock}),
		search.New(search.Settings{
			Allowed:       allowed,
			Ripgrep:       settings.Ripgrep,
			DefaultFolder: settings.workingDirectory(),
		}),
		shell.New(shell.Settings{
			Sandbox:          settings.Sandbox,
			Permission:       settings.Permission,
			Clock:            settings.Clock,
			Home:             settings.Home,
			NerdGenieProgram:     settings.NerdGenieProgram,
			WorkingDirectory: settings.workingDirectory(),
			Timeout:          settings.toolTimeout(),
		}),
		web.New(web.Settings{
			SearchServerAddress: settings.Configuration.SearchServerAddress,
			AllowedHosts:        settings.AllowedHosts,
			Timeout:             settings.toolTimeout(),
		}),
		memory.New(memory.Settings{Memory: settings.Memory}),
		task.New(task.Settings{Records: settings.Records}),
		skill.New(skill.Settings{Skills: settings.Skills}),
		job.New(job.Settings{Jobs: settings.Jobs}),
	}
	return append(tools, browserAndDesktopTools(settings)...)
}

// browserAndDesktopTools builds the seven browser tools and the desktop one, in
// the order the design lists them.
func browserAndDesktopTools(settings Settings) []contract.Tool {
	return []contract.Tool{
		browseropen.New(browseropen.Settings{Browser: settings.Browser}),
		browserread.New(browserread.Settings{Browser: settings.Browser}),
		browserclick.New(browserclick.Settings{Browser: settings.Browser}),
		browsertype.New(browsertype.Settings{Browser: settings.Browser}),
		browseract.New(browseract.Settings{Browser: settings.Browser}),
		browserlogin.New(browserlogin.Settings{
			Browser:       settings.Browser,
			Credentials:   settings.Credentials,
			TwoFactorCode: settings.TwoFactorCode,
		}),
		browserhandoff.New(browserhandoff.Settings{AskUser: settings.AskUser}),
		computer.New(computer.Settings{Desktop: settings.Desktop}),
	}
}
