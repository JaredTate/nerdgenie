package contract

// The names of the eighteen built-in tools, from design section 7. Every model
// sees all of them on every call, so the names are short and say what they do.
const (
	// ToolRead reads a file, a directory listing, or a past result by its id.
	ToolRead = "read"
	// ToolWrite creates a file or overwrites an existing one.
	ToolWrite = "write"
	// ToolEdit replaces one exact span of text in a file.
	ToolEdit = "edit"
	// ToolSearch finds files by name pattern, or lines by regular expression.
	ToolSearch = "search"
	// ToolShell runs a command in the sandbox and hands back a process id when
	// the command outlives ten seconds.
	ToolShell = "shell"
	// ToolWeb searches the web or fetches a public page as text.
	ToolWeb = "web"
	// ToolMemory searches, gets, or saves a memory.
	ToolMemory = "memory"
	// ToolTask is the only way the model writes to the task record.
	ToolTask = "task"
	// ToolSkill views, runs, or saves a skill.
	ToolSkill = "skill"
	// ToolJob creates a job with or without a schedule, adds a task to one, or
	// lists a job's tasks.
	ToolJob = "job"
	// ToolBrowserOpen opens a web page in the agent's own Chrome.
	ToolBrowserOpen = "browser_open"
	// ToolBrowserRead reads the current page as a compact tree.
	ToolBrowserRead = "browser_read"
	// ToolBrowserClick clicks one element and states what it expects to happen.
	ToolBrowserClick = "browser_click"
	// ToolBrowserType types into one element and states what it expects.
	ToolBrowserType = "browser_type"
	// ToolBrowserAct runs a short batch of browser steps and checks each one.
	ToolBrowserAct = "browser_act"
	// ToolBrowserLogin fills a login form from the vault without ever showing
	// the model the credentials.
	ToolBrowserLogin = "browser_login"
	// ToolBrowserHandoff brings the window forward and gives it to the user.
	ToolBrowserHandoff = "browser_handoff"
	// ToolComputer is the desktop tool, and the last resort when the browser
	// cannot do the job.
	ToolComputer = "computer"
)

// BuiltInToolNames returns the eighteen built-in tool names in the order design
// section 7 lists them.
func BuiltInToolNames() []string {
	return []string{
		ToolRead, ToolWrite, ToolEdit, ToolSearch, ToolShell, ToolWeb,
		ToolMemory, ToolTask, ToolSkill, ToolJob,
		ToolBrowserOpen, ToolBrowserRead, ToolBrowserClick, ToolBrowserType,
		ToolBrowserAct, ToolBrowserLogin, ToolBrowserHandoff, ToolComputer,
	}
}
