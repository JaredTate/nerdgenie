# The terminal screen: how it looks and how it behaves

This is the design for `internal/tui`, the terminal screen, so that the screen looks and feels like one piece of work rather than a pile of widgets. The screen is a thin client: it holds no state of its own, it draws what the running program sends over the local socket, and it sends back what the person types. Everything below is about how that drawing looks, and every rule in it is a test in `internal/tui`.

The screen is built on Bubble Tea, and the borders and the colour values are lipgloss's. Nothing else is used to draw it.

## Five rules

1. **Calm.** One ground, one text colour, one dim colour, one accent, one warning colour, and one error colour. The whole frame is painted on the ground so there are no dark gaps down the sides, and everything drawn on it is one of those five. No rainbow, no boxes inside boxes, no emoji in the frame. The reply is the loudest thing on the screen.
2. **Instant.** The first frame is drawn before the socket connects, and at the terminal's real size: `Run` asks the terminal how big it is with the `TIOCGWINSZ` request before Bubble Tea paints anything, and falls back to eighty by twenty-four. Nothing waits on the network to show the header and the input box.
3. **Honest about time.** A spinner appears only after five hundred milliseconds of waiting and, once shown, stays at least three seconds, so it never flickers. Streaming text appears as it arrives, coalesced every thirty milliseconds.
4. **Nothing hidden, nothing dumped.** Every tool call is one small pill. A result is never printed in full unless the person asks. A preview is the exact text or command that is about to run, in a card, with the answers under it as buttons.
5. **Never lie in the frame.** The status strip shows what the program said last: idle, thinking, using a tool, waiting for you, or disconnected. When the socket drops, the strip says so at once, the header says so too and keeps the model, the task and the cost it already knew, and the input box stays usable so the person can read back. And the frame is the screen's own drawing, never what it was handed: every piece of text that goes onto a row — what the person typed, what the program sent, what a model wrote — has its control characters turned into blanks first, because a screen that passes an escape character through lets whoever wrote it move the cursor, repaint the frame, or hide what it did, and a row holding a stray escape is no longer as wide as it measures. The only escape codes in the frame are the ones the screen adds itself, at the last step, and the picture protocols on a screenshot.


## The frame

Eighty columns is the design width. Below sixty it degrades by dropping the right side of the header and the key hints; it never wraps the header, and it never glues the right-hand piece onto the words beside it — a piece that will not fit with a real gap in front of it is dropped instead. Above one hundred and twenty it does not stretch; a bubble is never wider than one hundred columns of text and centres nothing.

```
 coeus · opus · task 17 running · 6.1k in 0.4k out · $0.04            ● healthy

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

- **Header, one row.** The wordmark `coeus`, then the link when there is none to speak of, the model alias, the task state, the cost of this session so far (tokens in and out, and money when the provider reports it), and a health dot on the right: a filled circle in the accent when the program answered its health check in the last ten seconds, hollow and dim otherwise, error-coloured when disconnected. Everything is dim except the task state, which is bold while a task runs, and the link word, which is dim while the screen has never reached the program ("connecting") and error-coloured once a link that was up has gone away ("disconnected"). A dropped link keeps the model, the task and the cost that were already known, because a person whose link went away still wants to know what was running.
- **A rule.** One thin line, dim.
- **The transcript.** Scrolls. The newest content is at the bottom and the view sticks to the bottom until the person scrolls up, then a small `▼ more` marker appears in the status strip until they return. Four kinds of block:
  - A **person's message**, in a bubble leaning against the right-hand edge of the frame: lipgloss's rounded border with the two-column bar `▎` as its left edge, filled with the accent so the words inside it are white on DigiByte blue. The bubble is only as wide as the words in it.
  - The **agent's reply**, streamed word by word, in a bubble leaning against the left-hand edge: the same rounded border, drawn dim, with nothing filled in, so the reply itself is the loudest thing on the row. Markdown is rendered lightly: bold, code spans, fenced code in a dim frame, lists with a dash. No headings larger than the text.
  - A **tool line**, one small filled pill per tool call, prefixed with `▸`: the tool name, its main argument, and a short result summary with the result id once it returns (`▸ read memory/product.md · 2,100 characters · r3`). Tool lines never show the result text; `/tasks 17` and `read r3` do that on purpose.
  - A **card**, for the four things that need the person: a preview (title `Ask me first`), a question (title `Question`), a handoff (title `Your turn in the browser`, with the screenshot inline where the terminal can draw pictures, else its file path), and an error (title `Something went wrong`, in the error colour, with what to do). A card is a single-line box with the title in the top rule. The preview's box and title are gold, because gold on the blue ground is the loudest pairing the palette has and the preview is the one card that must be answered. The answers are drawn as buttons on the last line inside the card, the key itself in a filled accent shape with its words beside it, all on one line where they fit and one to a line where they do not.
  - **The banner.** While the transcript is empty — the first frame, and any time there is nothing to show — the transcript area holds the wordmark instead: `COEUS AGENT` in a five-row block font written for this screen, centred, with one dim line under it saying "the agent that does not forget what it is doing", and under that a small filled tag naming the model alias and what the program is doing. It scrolls away the moment a conversation starts. A terminal too narrow for sixty-one columns of block letters gets the words `COEUS AGENT` in bold instead, because a wordmark cut in half is worse than a wordmark written small.
- **A rule.**
- **The input box.** Starts one row, grows to five as the person types, then scrolls inside. The prompt glyph is `›` in accent. Enter sends, Ctrl+J inserts a newline, Up recalls the last message, Esc while a task runs sends stop, Esc at a card or a masked prompt withdraws from it, Ctrl+C twice quits. Typing `/` at the start opens the command palette: a dim list above the input of every registered command with its one-line help, filtered as the person types, Tab or Enter to complete.
- **The status strip, one row.** Left: the state in plain words (`idle`, `thinking`, `using read`, `waiting for you`, `paused`, `disconnected, reconnecting`), then the budget line during a task and a four-cell bar showing how much of it is left. Right: three key hints that change with the state, dropped when there is no room for them with a gap in front. When a spinner is due, it sits at the far left of this row, never in the transcript.

**The budget bar.** The program reports its budget in plain words, such as "86 rounds, 51 min left", so the screen reads the first number in that line as the count and measures it against the largest count it has seen since this task started. A new task starts the measure again. There is no fuller measure to be had, and a bar measured against the fullest report of this task is the truth as the screen knows it.

## The masked prompt

When the program asks for a secret, the input box switches to secret mode: the prompt glyph becomes `🔒` where the terminal has it and `*` where it does not, the title of the request is shown dim above the box (`API key for anthropic`), every typed character is drawn as `•`, paste works, Enter sends, Esc withdraws. Nothing typed in secret mode goes into the transcript, the history, or the log. The test types a secret and asserts the whole frame contains only bullets.

Esc at a masked prompt, and Esc at a card waiting for an answer, both send `contract.SocketCancel` with the id of the thing being withdrawn from, not a `deny`. The person refused nothing; they closed a box, and the program has to know at once rather than waiting out its whole deadline. A rejection, which is a real no, still travels as a `deny` with the person's reason in `Reason`.

## Pictures

Screenshots from a handoff or `/screen` are drawn inline when the terminal supports the kitty graphics protocol or the iTerm2 inline image protocol (detected from the environment, never assumed), scaled to the transcript width. The file is read and encoded once, when the card is made, not on every frame it is drawn in. Otherwise the card shows the file path and the words "open this file to see the page", and the path is selectable. A `reply` that carries files names each of them as a pill, because a file whose path is nowhere on the screen is a file the person cannot open.

## Colours

The palette is DigiByte's, and the frame paints its own ground rather than borrowing the terminal's:

| Name | Value | What it draws |
|---|---|---|
| ground | `#1E90FF` | The whole frame, every row, edge to edge |
| text | `#FFFFFF` | Every ordinary word |
| dim | `#CFE6FF` | The header, the rules, the status strip, the agent's bubble border |
| accent | `#0066CC` | The prompt glyph, the health dot, and the fill of the pills, the buttons and the person's bubble |
| warning | `#FFD166` | The border and the title of the card the person must answer |
| error | `#FF6B6B` | A failure, a dropped link, and the error card |

The colours are written as lipgloss colour values and stepped down to whatever the terminal has: twenty-four bit when `COLORTERM` says `truecolor` or `24bit` or `TERM` says `direct`, the two hundred and fifty-six colour cube when `TERM` names `256color`, the sixteen ordinary colours otherwise, and no colour at all when `NO_COLOR` is set or when the terminal says nothing about itself. `NO_COLOR` turns every colour off and the frame must still make sense: the bar `▎`, the arrow `▸`, the rules, the rounded bubbles and the card borders carry the structure without colour, and the plain frame is the coloured frame with the escape codes taken out, letter for letter. That is a test.

The escape codes are written by the screen rather than by `lipgloss.Style.Render` for one reason: a lipgloss renderer reports "no colour at all" whenever the writer it was built on is not a terminal, which every test process is, so painting through it would drop the colours out of exactly the golden files that have to prove them. lipgloss owns the colour values and the border glyphs; the screen owns the depth.

## Motion

- Deltas are appended to the reply block as they arrive, coalesced every thirty milliseconds so the terminal is not repainted per word.
- The spinner is a four-frame dot cycle (`·  `, ` · `, `  ·`, ` · `) at eight frames a second, drawn only in the status strip.
- Resize re-wraps the transcript and redraws once; the view keeps its position relative to the bottom. No clearing of the screen, no flash.
- A new card scrolls into view and takes focus; the input box is dimmed until the card is answered or dismissed, and the status strip says `waiting for you`. The card that holds the single keys is remembered by the number it was given when it was shown, so that anything arriving above it — an error card most of all — cannot take its answers away.

## What the tests check

- The golden first frame at eighty by twenty-four before any socket message: the header with `coeus · connecting`, the rule, the banner, the rule, the input box, and the strip saying `connecting`.
- Golden frames, themed and plain, at eighty by twenty-four and at one hundred and twenty by forty, for the banner and for a conversation with a bubble, a pill and a card in it. Golden frames at sixty and sixty-eight columns for the narrow degrade.
- Every row of a themed frame is painted to the full width of the terminal and carries a background colour, so there are no dark gaps.
- The spinner appears at five hundred milliseconds on the fake clock and is still there at three seconds even when the reply arrived at one second.
- An approval card answered with `a`, `A`, and `r` sends the right socket message and the transcript records the answer in one pill, and it still does with an error card sitting on top of it.
- The masked prompt frame contains only bullets.
- Resizing from eighty to sixty and back produces the same transcript text with different wrapping and no leftover characters.
- `NO_COLOR` renders the same structure, and the design's own numbers — the thirty-millisecond heartbeat, the spinner's rate, delay and hold, the hundred-column wrap, the five-row input box, the ten-second health window — are pinned as literals.

## What the person judges in the trial

Whether the first frame is immediate and the right size, whether streaming looks smooth, whether the preview card is obvious and the answers are discoverable, whether the masked prompt is trustworthy, and whether anything flickers on resize. Anything confusing, slow, or ugly becomes a brief.
