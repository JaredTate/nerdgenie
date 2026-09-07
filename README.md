# Nerd Genie

Nerd Genie is an open-source AI agent that runs on your own Linux machine. You talk to it in a terminal or over Signal on your phone. It works with any language model, big or small, running on your own graphics card or in the cloud, and it does not forget what it is doing, because it keeps a task record instead of re-reading its whole conversation every turn. It drives a real Chrome window, and with a model that can see, it looks at the pages and the games it builds.

**Start here.** `INSTALL.md` gets the program and the local model onto a machine. `SETUP.md` says how to configure it, open the screen, give it work, and read the screen. `PROMPT_TEMPLATE_GUIDE.md` says how to write an ask it can lift, with six worked examples beside it. `NERDGENIE.md` is the plain-words explanation of how it works and why, with a check table that ties every claim to a test.

## What it does today

- Builds and tests software on its own for hours: a long ask becomes a job of tasks, each with its own record, run one at a time with a report after each.
- Uses the browser like a person: opens pages, reads them as an outline, acts by reference, checks that what it expected happened, and reports a page that hangs by the loop that never yields.
- Sees: with the vision projector loaded, screenshots and picture files reach the model as images, about 800 tokens each.
- Keeps its own honest state: a task record shaped like an Army operations order, an append-only event log, and a working context built fresh each call and sized to the model.
- Watches itself: a progress meter that nudges, rewinds, and stops; a stuck-test line; refusals that say what to write instead; a nightly set of asks measured by a run report.
- Runs on a 27B model on one graphics card: on this machine, Qwen 3.8 through the TurboQuant llama-server with MTP, about 420 tokens a second of prefill and 60 to 75 of output, and a 90 percent cache share on real work. The same harness drives Opus through Claude Code, GPT through Codex, and any model behind an OpenAI-compatible server such as Ollama.
- Reads a project's own documents the way a person does: `AGENTS.md` in the work folder rides in front of every task, and the sections of `ARCHITECTURE.md` and the roots of `REPO_MAP.md` are named at every fresh start so the model reads one section by name instead of a whole file.

## The idea

An AI agent is a language model with a program wrapped around it. The model is a simple machine. Text goes in, text comes out, and it remembers nothing from one call to the next. The program around it is called the harness. The harness gives the model the two things it lacks: a memory, and hands. The hands are tools, such as reading a file, running a command, or opening a web page. Everyone can use the same models, so the harness is what makes one agent different from another.

Every agent we studied uses the conversation transcript as its record of the task. The transcript is the full text of everything said so far. On each turn the model re-reads the whole thing to work out where it is. When the transcript grows too long, the agent squeezes it into a summary and hopes nothing important was lost. Picture a video game that loads your saved game by replaying every button you ever pressed since you started. Re-reading the transcript is that replay, and you pay for it on every turn.

Nerd Genie keeps three things apart, which is an old idea from computer science called state.

- **History** is what happened: every message, every tool call, every result. New lines are added at the end and old lines are never changed. It is not put in front of the model by default.
- **State** is what is true right now. It is a task record of one to three thousand tokens, and it is always in front of the model.
- **Working context** is what the model looks at during one call. It is built fresh each turn and sized to the model.

Picture a writer at work. The library holds everything ever written, and that is the history. The desk holds the few books open for today's chapter, and that is the state. The page in front of the writer is the working context.

## The task record, borrowed from the Army

The task record is shaped like the United States Army's five-paragraph operations order, a fixed format that anyone can write and anyone can check. It holds the user's request word for word, never edited. It holds what the user wants and what "done" looks like. It holds the situation as the world reports it right now, the plan, every decision with its reason, every failure with its cause, and a short list of conditions that mean "stop and tell the user." A correction from the user is added to the record in the user's own words, the way the Army sends a short fragmentary order that changes one part of a plan without restating the rest. When the task ends, the agent runs a done-check against the record and then answers four after-action questions, and what it learns goes into memory or becomes a skill.

The result is an agent that always knows what it is working on. It keeps working until the job is done. It stops and tells you when it is supposed to. And it spends its tokens on the task instead of on re-reading its own past.

## Any model, big or small

Nerd Genie has one rule for the working context. It is never smaller than the task needs and never bigger than the model can hold. The task record is the same size on every model. Only the window around it changes. On a small model running on your own machine, the window is a few thousand tokens. On a frontier model, it is most of a million. A big model is never held back to suit a small one, and a small model is never asked to hold more than it can.

The prompt is built in layers, from the part that changes least to the part that changes most, because model providers charge about a tenth as much for text they have already read. The persona, which says who the agent is and who you are, almost never changes. A skill changes only when a website or a tool changes. The task changes every turn. Keeping the three apart is what makes every turn cheap.

## The browser, used like a human

Nerd Genie launches a real Chrome web browser with its own user profile, never your daily one. It does not use a website's API. It looks at the page, acts on it at the speed a person would, and checks that the action did what it expected before moving on. When it hits a login wall, a two-factor prompt, or a captcha, it brings the window to the front and hands off to you. It can learn a browser skill by watching you do something once or by reading the documentation, and it replays that skill later without calling the model at all. It can also use the Linux desktop and any command-line tool the same way, and it can visually check an app on screen and report what it sees.

## Safe by design

Nothing irreversible happens without a preview you approve first. Commands run inside a sandbox, a fenced-off area of the computer that cannot reach your keys or the vault. Passwords live in an encrypted vault, entered only through a masked prompt in the terminal, and the model never sees them; it asks the harness to log in for it. One permission function decides every action, and when no rule matches, it asks you. Everything is written to the log.

## Day to day

- Talk to it in the terminal or on Signal. The same slash commands work in both.
- `/tasks` shows what it is working on. `/tasks 17 back 3` rewinds a task three checkpoints, like reloading a saved game.
- `/jobs` lists every job with its progress. `/cron` lists the ones with a schedule, in plain words, with when each last ran and when it runs next.
- `/memory` and `/skills` show what it remembers and what it has learned. Your corrections are kept word for word.
- `/undo` reverts the last turn's file changes. `/status` shows the model, the cost, and the health.
- `nerdgenie update` installs a new release and rolls back by itself if the new one does not come up.

## Built to grow

The core, meaning the loop, the task record, the permissions, and the memory, is closed. Around it are four fixed shapes that can multiply: channels, tools, skills, and model providers. The first version has the terminal and Signal, eighteen tools, and three providers: Anthropic's API, the OpenAI-compatible API that every local model server and gateway speaks, and a command-line provider that drives Claude Code or Codex on a subscription. Adding Telegram later is one new file that fits the channel shape. Adding your own tool is one executable in a folder.

## How it gets built

Tests are written before the code they prove, at four levels: unit, integration, functional, and fuzz. The agent is written in Go, and the browser and desktop workers in TypeScript. Nothing lands until every test is green against a fake model, and the live suite runs the same tasks against real ones: a local Qwen, Opus through Claude Code, and GPT through Codex. `TESTING.md` says how it is tested and `CONTRIBUTING.md` how to change it.

## Files

| File | What it is |
|---|---|
| `INSTALL.md` | Getting the program, the model server and the model files onto a machine |
| `SETUP.md` | The home folder, the configuration, the screen, the browser, giving it work, measuring it, and what to do when something looks wrong |
| `PROMPT_TEMPLATE_GUIDE.md` | How to write an ask the harness can lift: six headings, what the harness does with each, and how the model fetches a project's documents when it needs them |
| `EX_PROMPT_1_TIC_TAC_TOE.md` to `EX_PROMPT_6_FLIGHT_SIM.md` | Six worked asks in that shape, numbered from easiest to hardest, each with its difficulty and expected time |
| `NERDGENIE.md` | The plain-words explanation: the problem every agent has, the three things Nerd Genie keeps apart, the task record, the four kinds of state, one turn, small and big models, why it uses fewer tokens and remembers better, what we took and what is new, the tools, and a check table tying every claim to a design section and a test |
| `ARCHITECTURE.md`, `REPO_MAP.md`, `CLAUDE.md` | How the code is put together, where everything lives (generated), and the rules an AI agent working on this repository follows |
| `TESTING.md`, `CONTRIBUTING.md` | How it is tested, and how to change it |
| `docs/NERDGENIE_PLAN.md` | The design: the idea, what we learned from other agents, how the agent works, the four kinds of state and the task record, what the model is told, tools, browser, safety |
| `docs/HARNESS_V2.md` | The comparison of OpenClaw 2.0, Hermes, Prime, OpenCode, Atomic, ZeroClaw, Codex, and Claude Code, one diagram each |
| `docs/THIRD_PARTY.md` | The projects whose designs were borrowed, and their licenses |

Diagrams are mermaid blocks. In VS Code, open the preview with the "Markdown Preview Mermaid Support" extension.
