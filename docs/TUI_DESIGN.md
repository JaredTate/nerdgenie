# The terminal screen: how it looks and how it behaves

This is the design for `internal/tui`, the terminal screen, so that the screen looks and feels like one piece of work rather than a pile of widgets. The screen is a thin client: it holds no state of its own, it draws what the running program sends over the local socket, and it sends back what the person types. Everything below is about how that drawing looks, and every rule in it is a test in `internal/tui`.

The screen is built on Bubble Tea, version 2, and on nothing else: the borders and the colour values are the screen's own. Version 1 asked the terminal for its background colour while its package was being set up, before any code of ours could run, and waited five seconds for each byte of the answer, swallowing what the person typed on a terminal that never answers; version 2 dropped that question, and dropped the styling library underneath that asked it.

## Five rules

1. **Calm.** One ground, one text colour, one dim colour, one accent, and one green for check marks: five colours, named once. The whole frame is painted on the ground, and the terminal itself is asked to take the ground as its own background while the program runs, so there are no dark gaps anywhere, and everything drawn on it is one of those five. Colour never carries a meaning alone: a glyph carries it too, so a terminal with no colour reads the same frame. No rainbow, no boxes inside boxes, no emoji in the frame, nothing that blinks. The reply is the loudest thing on the screen.
2. **Instant.** The first frame is drawn before the socket connects, and at the terminal's real size: `Run` asks the terminal how big it is with the `TIOCGWINSZ` request before Bubble Tea paints anything, and falls back to eighty by twenty-four. Nothing waits on the network to show the header and the input box.
3. **Honest about time.** A spinner appears only after five hundred milliseconds of waiting and, once shown, stays at least three seconds, so it never flickers. Streaming text appears as it arrives, coalesced every thirty milliseconds.
4. **Nothing hidden, nothing dumped.** Every tool call is one small pill. A result is never printed in full unless the person asks. A preview is the exact text or command that is about to run, in a card, with the answers under it as buttons.
5. **Never lie in the frame.** The status strip shows what the program said last: idle, thinking, using a tool, waiting for you, or disconnected. When the socket drops, the strip says so at once, the header says so too and keeps the model, the task and the cost it already knew, and the input box stays usable so the person can read back. And the frame is the screen's own drawing, never what it was handed: every piece of text that goes onto a row — what the person typed, what the program sent, what a model wrote — has its control characters turned into blanks first, because a screen that passes an escape character through lets whoever wrote it move the cursor, repaint the frame, or hide what it did, and a row holding a stray escape is no longer as wide as it measures. The only escape codes in the frame are the ones the screen adds itself, at the last step, and the picture protocols on a screenshot.


## The frame

Eighty columns is the design width. Below sixty it degrades by dropping the right side of the header and the key hints; it never wraps the header, and it never glues the right-hand piece onto the words beside it — a piece that will not fit with a real gap in front of it is dropped instead. Above one hundred and twenty it does not stretch; a bubble is never wider than one hundred columns of text and centres nothing. At a hundred columns and wider there is room for the side panel below, and everything between the two rules is drawn in what is left; below a hundred the panel is dropped and nothing else changes.

```
 NERDGENIE · opus · task 17 running · 6.1k in 0.4k out · $0.04            ● healthy

     ╭────────────────────────────────────────────────────────────────────────╮
     ▎ Post a tweet about the DigiByte anniversary. Use the product notes and │
     ▎ keep it under 280 characters.                                          │
     ╰────────────────────────────────────────────────────────────────────────╯

 ╭───────────────────────────────────────────────────╮
 │ Where I stand: the notes are read, drafting next. │
 ╰───────────────────────────────────────────────────╯
    ▸ read memory/product.md · 2,100 characters · r3

 ┌ Ask me first ──────────────────────────────────────────────────────────────┐
 │ browser_click e7 "Post"                                                    │
 │                                                                            │
 │ [ a ] approve once  [ A ] always this session  [ r ] reject with a reason  │
 └────────────────────────────────────────────────────────────────────────────┘
 ──────────────────────────────────────────────────────────────────────────────
 › _
 waiting for you · 86 rounds, 51 min left ▰▰▰▰  a approve · A always · r reject
```

From top to bottom:

- **Header, one row.** The wordmark, `NERD` in bold white and `GENIE` in bold DigiByte blue, then ` · the open-source agent harness` dim, then the link when there is none to speak of, the model alias, the context measure, the task in words — `task 24 running` when the program said what state it is in, and `task 24` when it did not, never the number on its own, because a bare number in the middle of the header says nothing — the cost of this session so far (tokens in and out, and money when the provider reports it), and a health dot on the right: a filled circle in the accent when the program answered its health check in the last ten seconds, hollow and dim otherwise, hollow in bold white when disconnected. Everything after the wordmark is dim except the task state, which is bold while a task runs, and the link word, which is dim while the screen has never reached the program ("connecting") and bold white once a link that was up has gone away ("disconnected"). The tagline is a nicety and the status is information, so a row too narrow for both drops the tagline first and only then cuts the status. A dropped link keeps the model, the task and the cost that were already known, because a person whose link went away still wants to know what was running.
- **A rule.** One thin line, dim.
- **The transcript.** Scrolls. The newest content is at the bottom and the view sticks to the bottom until the person scrolls up: one notch of the mouse wheel, Shift+Up or Shift+Down moves it three rows, Page Up or Page Down ten, and it stops at the oldest row and at the newest. The screen asks the terminal to report the mouse cell by cell, which is what makes a terminal send the wheel, and turns that off again when it quits, so the shell is left as it was found. While the view is scrolled up a small `↑ older` mark sits in the status strip, so that the person knows why new text is not appearing, and new output does not pull the view down: a reply, a streamed piece of one, or a tool line arriving underneath leaves the rows being read exactly where they are. The view comes back to the newest row when the person wheels or keys down to the bottom, when they send a message, and when a card arrives that needs them. A block that would fit in a transcript of its own but not in the room left at the top is not drawn at all while the view rests on the newest row, because a bubble cut in two is worse than a bubble not shown, in the same way that a wordmark cut in half is worse than a wordmark written small; a view scrolled up is a window moved by rows, and the blocks at its edges are cut, because a view moving three rows at a time has to cross every block on its way. A block taller than the whole transcript is the other exception: it is drawn and cut, because there is no room for it whole anywhere and its newest rows are the ones being read. Four kinds of block:
  - A **person's message**, in a bubble leaning against the right-hand edge of the frame: a rounded border with the two-column bar `▎` as its left edge, filled with the accent so the words inside it are white on DigiByte blue. The bubble is only as wide as the words in it.
  - The **agent's reply**, streamed word by word, in a bubble leaning against the left-hand edge: the same rounded border, drawn dim, with nothing filled in, so the reply itself is the loudest thing on the row. Markdown is rendered lightly: bold, code spans, fenced code in a dim frame, lists with a dash. No headings larger than the text.
  - A **tool line**, one small filled pill per tool call, prefixed with `▸`: the tool name, its main argument, and a short result summary with the result id once it returns (`▸ read memory/product.md · 2,100 characters · r3`). Tool lines never show the result text; `/tasks 17` and `read r3` do that on purpose. One call is one pill, however many times the program says so: the program sends the line for the call in flight on every heartbeat until the call changes, a line that is already on the screen draws nothing new, and the same call's line with its result added takes the place of the pill that call already has rather than making a second one. The program's line already begins with the arrow, so the screen takes that arrow off before drawing its own; a pill never reads `▸ ▸`. The same call made again straight after the last one came back — the same tool with the same argument, which is what a model that has got stuck does, and the trial saw one shell command fill the screen thirteen times over — is not a new pill: the pill it already has takes the newest line and a count on the end, `▸ shell make test · r13 shell: 12 lines × 13`, because thirteen rows saying one thing tell the person less than one row saying how many times. A heartbeat carrying the same line is one call, never a count, and a different call between two of the same keeps them apart.
  - A **card**, for the four things that need the person: a preview (title `Ask me first`), a question (title `Question`), a handoff (title `Your turn in the browser`, with the screenshot inline where the terminal can draw pictures, else its file path), and an error (title `Something went wrong`, in bold white, with what to do). A card is a single-line box with the title in the top rule. The preview's box and title are the accent, the colour of everything on the frame that asks to be acted on, because the preview is the one card that must be answered; an error card's box and title are bold white, the loudest thing the palette has, and its title says what went wrong. The answers are drawn as buttons on the last line inside the card, the key itself in a filled accent shape with its words beside it, all on one line where they fit and one to a line where they do not.
  - **The welcome.** While the transcript is empty — the first frame, and any time there is nothing to show — the transcript area holds the welcome instead, centred: the wordmark `NERD GENIE` in a five-row block font written for this screen, fifty-five columns wide, `NERD` in white and `GENIE` in DigiByte blue; under it the tagline `the open-source agent harness` dim, then `your wish is its command.` in dim italics, then a blank line and `type an ask, or /help` dim, which is the one line on a first frame that says how to start and where the commands are. Nothing else is drawn there: the model and the state belong to the header and the strip. The block letters are drawn only when the transcript area is at least sixty columns by fourteen rows; a smaller area gets the wordmark on one line, in the same two colours, because a wordmark cut in half is worse than a wordmark written small. There is no mascot and no genie in ASCII: block letters are the most a monospace grid draws well. The welcome scrolls away the moment a conversation starts.
- **A rule.**
- **The input box.** Starts one row, grows to five as the person types, then scrolls inside. The prompt glyph is `›` in accent. Enter sends, Ctrl+J inserts a newline, Up recalls the last message, Esc while a task runs sends stop, Esc at a card or a masked prompt withdraws from it, Page Up, Page Down, Shift+Up and Shift+Down scroll the transcript whoever holds the other keys, Ctrl+C twice quits. Typing `/` at the start opens the command palette: a dim list above the input of every registered command with its one-line help, filtered as the person types, Tab or Enter to complete.
- **`/clear`.** The one command the screen itself acts on. The program stops the task that is running, forgets that the terminal had a task to carry on, so that the next message starts a fresh task rather than answering a question the person can no longer see, and answers with a reply carrying `Clear`. A reply with `Clear` empties the transcript and any reply still on its way, and then shows its own one line, `cleared: the next message starts a fresh task`. The header, the status strip and the side panel keep what they said, because they are drawn from what the program knows about itself; and the tool line and the record line the screen last drew are remembered still, because the program sends them again on every heartbeat and a screen that forgot them would draw them straight back into the empty transcript.
- **The status strip, one row.** Left: the state in plain words (`idle`, `thinking`, `using read`, `waiting for you`, `paused`, `disconnected, reconnecting`), then the budget line during a task and a four-cell bar showing how much of it is left. Right: three key hints that change with the state, dropped when there is no room for them with a gap in front. When a spinner is due, it sits at the far left of this row, never in the transcript.

**The budget bar.** The program reports its budget in plain words, such as "86 rounds, 51 min left", so the screen reads the first number in that line as the count and measures it against the largest count it has seen since this task started. A new task starts the measure again. There is no fuller measure to be had, and a bar measured against the fullest report of this task is the truth as the screen knows it.

## The side panel: the checklist

At a hundred columns and wider, the last twenty-eight columns between the two rules are a panel down the right-hand side, drawn the way opencode draws its sidebar: a thin dim line down its left edge, a blank column after it, and short quiet lines in groups with a blank line between them. It holds what a person wants to see without asking for it, and nothing the program has not said: the model and what it has cost, then one checklist of the work, then how many jobs are waiting.

```
 NERDGENIE · opus · ctx 12.4k / 262k · 5% · task 17 running · 6.1k in 0.4k out · $0.04            ● healthy
 ─────────────────────────────────────────────────────────────────────────────────────────────────────
                                                             │ opus
                                                             │ ctx 12.4k / 262k · 5%
                                                             │ 6.1k in 0.4k out · $0.04
                                                             │
                                                             │ JOB 4 · Tater Tots Tetris
                                                             │ ─────────────────────────
                                                             │ ✓ t17 post the anniversa…
                                                             │ ▶ t19 draft the blog pie…
                                                             │   ✓ the product notes ar…
                                                             │   ✓ a draft under 280 ch…
                                                             │   ▶ the tweet is posted
                                                             │ ○ t22 post for day two
                                                             │ ─────────────────────────
                                                             │ 1 of 3 done
```

The first group is quiet: the model alias, the context measure with its share, and the session's tokens and money, one dim line each and no box around them. The share is the header's own piece, so it turns the accent and then bold white at the same places.

The checklist is one block with one hierarchy, top to bottom. Its header names the work: `JOB 4 · Tater Tots Tetris` while a job's task is running, the word in bold white, the number in the accent, the name in bold white, and `TASK 17` while a plain task runs; a job made without a name has its ask on a dim line under its number instead. Then a thin rule in DigiByte blue. Then one row per task of the job, in order, at most twelve of them and a dim line saying how many more there are: the glyph of the task's state, its label (`t17`) dim, and its words, white on the running task and dim on every other, cut to the panel with an ellipsis, because a checklist is one row per item. The glyphs carry the state on their own, so a terminal with no colour reads the list: a green `✓` on a task that is done, a `▶` in DigiByte blue on the task running now, a dim `○` on a task still to come. Under the running task, and under it alone, its plan steps are drawn two columns in with the same glyphs — the check on each finished step, the pointer on the first that is not, because that is where the work stands inside the task — at most eight of them and a line saying how many more there are. A plain task shows its plan steps the same way straight under its header. Then a rule, and `1 of 3 done` in the accent, counting the job's tasks or the plain task's steps. Every row sits on the same column grid.

The last group is how many jobs are waiting, `3 jobs waiting`, dim, drawn while no job is on the checklist. A group the program has said nothing about is left out altogether, blank line and all, so a panel beside an idle agent is the model alone. The checklist is built from the status fields for the job, its name, its ask, its running task, its task list, and the running task's plan; the jobs line from the count of jobs; everything else in the panel is what the header already knows. The status carries no ask for a plain task, only its number, so a plain task's header is the number alone until the program sends one.

## The masked prompt

When the program asks for a secret, the input box switches to secret mode: the prompt glyph becomes `🔒` where the terminal has it and `*` where it does not, the title of the request is shown dim above the box (`API key for anthropic`), every typed character is drawn as `•`, paste works, Enter sends, Esc withdraws. Nothing typed in secret mode goes into the transcript, the history, or the log. The test types a secret and asserts the whole frame contains only bullets.

Esc at a masked prompt, and Esc at a card waiting for an answer, both send `contract.SocketCancel` with the id of the thing being withdrawn from, not a `deny`. The person refused nothing; they closed a box, and the program has to know at once rather than waiting out its whole deadline. A rejection, which is a real no, still travels as a `deny` with the person's reason in `Reason`.

## Pictures

Screenshots from a handoff or `/screen` are drawn inline when the terminal supports the kitty graphics protocol or the iTerm2 inline image protocol (detected from the environment, never assumed), scaled to the transcript width. The file is read and encoded once, when the card is made, not on every frame it is drawn in. Otherwise the card shows the file path and the words "open this file to see the page", and the path is selectable. A `reply` that carries files names each of them as a pill, because a file whose path is nowhere on the screen is a file the person cannot open.

## Colours

The palette is DigiByte's, five colours named once in `style.go` and nothing else; a test reads every source file of the package for a colour written as hex digits and fails on any other. The frame paints its own ground on every row, and for as long as the program runs the terminal itself is asked to take the ground as its own background and white as its own foreground (Bubble Tea sends the OSC 11 and OSC 10 colour requests for `tea.View.BackgroundColor` and `ForegroundColor`, and sends the resets when the screen quits), so every cell is dark blue whether or not a row painted it, and the shell gets its own colours back:

| Name | Value | What it draws |
|---|---|---|
| ground | `#002352` | DigiByte dark blue: the whole frame, every row, edge to edge, and the terminal's own background while the program runs |
| text | `#FFFFFF` | Every ordinary word, and the wordmark's `NERD`; bold, it is the loudest thing on the ground: the checklist's header, a failure, a dropped link |
| accent | `#0066CC` | DigiByte blue: the wordmark's `GENIE`, the checklist's rules, the `▶` beside the running task, and the fill of the pills, the buttons and the person's bubble |
| accent text | `#4DA3FF` | The same blue lightened for small text, because the pure blue is too dark to read at that size on the ground: the prompt glyph, the health dot, a job's number, `1 of 3 done`, the context share once it is filling up, and the border and title of the card the person must answer |
| dim | `#8FA9CC` | The header's tagline and status, the rules, the status strip, the agent's bubble border, every label, and everything on the checklist that is not where the work is |
| done | `#3DDC84` | Green, for the `✓` beside something finished and for nothing else |

The colours are written as six hex digits each and stepped down to whatever the terminal has: twenty-four bit when `COLORTERM` says `truecolor` or `24bit` or `TERM` says `direct`, the two hundred and fifty-six colour cube when `TERM` names `256color`, the sixteen ordinary colours otherwise, and no colour at all when `NO_COLOR` is set or when the terminal says nothing about itself. `NO_COLOR` turns every colour off and the frame must still make sense: the bar `▎`, the arrow `▸`, the checklist's `✓`, `▶` and `○`, the rules, the rounded bubbles and the card borders carry the structure without colour, and the plain frame is the coloured frame with the escape codes taken out, letter for letter. That is a test.

The escape codes are written by the screen rather than by a terminal styling library for one reason: such a library reports "no colour at all" whenever the writer it was built on is not a terminal, which every test process is, so painting through one would drop the colours out of exactly the golden files that have to prove them. The screen owns the colour values, the border glyphs, and the depth.

## Motion

- Deltas are appended to the reply block as they arrive, coalesced every thirty milliseconds so the terminal is not repainted per word.
- The spinner is a four-frame dot cycle (`·  `, ` · `, `  ·`, ` · `) at eight frames a second, drawn only in the status strip.
- Resize re-wraps the transcript and redraws once; the view keeps its position relative to the bottom. No clearing of the screen, no flash.
- The mouse wheel and the scroll keys move the view by rows, and a view scrolled up is held on the rows it was reading while new output arrives underneath; only the person's own message, a card that needs them, or a scroll back to the bottom brings it down.
- A new card scrolls into view and takes focus; the input box is dimmed until the card is answered or dismissed, and the status strip says `waiting for you`. The card that holds the single keys is remembered by the number it was given when it was shown, so that anything arriving above it — an error card most of all — cannot take its answers away.

## What the tests check

- The golden first frame at eighty by twenty-four before any socket message: the header with `NERDGENIE · the open-source agent harness · connecting`, the rule, the welcome, the rule, the input box, and the strip saying `connecting`.
- Golden frames, themed and plain, at eighty by twenty-four and at one hundred and twenty by forty, for the welcome and for a conversation with a bubble, a pill and a card in it. Golden frames at sixty and sixty-eight columns for the narrow degrade, and at one hundred and twenty by thirty-six for the side panel: idle, with a plain task, and with a named job.
- The palette: every source file of the package holds no colour but the five; each style draws the colour it is named for; the ground and the accent land on different colours at sixteen and at two hundred and fifty-six; `View` asks the terminal for the ground and white and asks a `NO_COLOR` terminal for nothing; and the real screen, run on a pseudo-terminal that promises true colour, sends the OSC 11 request for `#002352`, the palette's own colour codes, and the resets when it is quit with Ctrl+C twice.
- The checklist: a glyph per state, in its colour; the running task's plan indented under it and under no other; a plain task's plan under its header with its progress and the waiting-jobs line after it; a long row cut with an ellipsis and never past the panel; `JOB 4 · Tater Tots Tetris` in the palette's colours; twelve tasks and eight steps at most.
- The wordmark and the welcome: `NERD` white and `GENIE` blue on the header and in the block letters; the block wordmark exactly fifty-five columns and five rows; drawn at sixty by fourteen and not at fifty-nine by fourteen or sixty by thirteen; the tagline dropped from the header before the status is cut.
- Every row of a themed frame is painted to the full width of the terminal and carries a background colour, so there are no dark gaps.
- The spinner appears at five hundred milliseconds on the fake clock and is still there at three seconds even when the reply arrived at one second.
- An approval card answered with `a`, `A`, and `r` sends the right socket message and the transcript records the answer in one pill, and it still does with an error card sitting on top of it.
- The masked prompt frame contains only bullets.
- Resizing from eighty to sixty and back produces the same transcript text with different wrapping and no leftover characters.
- A reply carrying `Clear` on a real frame leaves nothing of the conversation and keeps the header, the strip and the panel; the same tool line on the next heartbeat does not refill it; and the same call made thirteen times in a row is one pill reading `× 13`.
- `NO_COLOR` renders the same structure, and the design's own numbers — the thirty-millisecond heartbeat, the spinner's rate, delay and hold, the hundred-column wrap, the five-row input box, the ten-second health window — are pinned as literals.
- Scrolling, on real frames: one notch of the wheel up shows the view three rows older and one notch down three rows newer; a hundred notches stop at the oldest row and at the newest; a reply, a streamed reply and a tool line arriving while scrolled up leave the rows in view exactly where they were; wheeling down to the bottom and sending a message both show the newest rows again; Page Up, Page Down, Shift+Up and Shift+Down do the same by the keyboard, a plain Up still recalls the last message, and Shift+Up still scrolls while a card holds the keys; the golden frame at eighty by twenty-four scrolled two notches up carries the `↑ older` mark; and the whole program, run through `Run` on a pipe with a fake link, scrolls on the bytes a terminal sends for the wheel and has mouse reporting turned off by the time it quits.

## What the person judges in the trial

Whether the first frame is immediate and the right size, whether streaming looks smooth, whether the preview card is obvious and the answers are discoverable, whether the masked prompt is trustworthy, and whether anything flickers on resize. Anything confusing, slow, or ugly becomes a brief.
