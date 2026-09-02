# The terminal screen: how it looks and how it behaves

This is the design for `internal/tui`, the terminal screen, written by the orchestrator so that the screen looks and feels like one piece of work rather than a pile of widgets. Brief 3.4 builds it, and the human trial after wave 3 judges it against this document. The screen is a thin client: it holds no state of its own, it draws what the running program sends over the local socket, and it sends back what the person types. Everything below is about how that drawing looks.

## Five rules

1. **Calm.** One accent color, one dim color, one error color, and the terminal's own foreground. No rainbow, no boxes inside boxes, no emoji in the frame. The reply is the loudest thing on the screen.
2. **Instant.** The first frame is drawn before the socket connects. Nothing waits on the network to show the header and the input box.
3. **Honest about time.** A spinner appears only after five hundred milliseconds of waiting and, once shown, stays at least three seconds, so it never flickers. Streaming text appears as it arrives, coalesced every thirty milliseconds.
4. **Nothing hidden, nothing dumped.** Every tool call is one dim line. A result is never printed in full unless the person asks. A preview is the exact text or command that is about to run, in a box, with the three answers under it.
5. **Never lie in the frame.** The status strip shows what the program said last: idle, thinking, using a tool, waiting for you, or disconnected. When the socket drops, the strip says so at once and the input box stays usable so the person can read back.

## The frame

Eighty columns is the design width. Below sixty it degrades by dropping the right side of the header and the key hints; it never wraps the header. Above one hundred and twenty it does not stretch; the transcript wraps at one hundred columns and centers nothing.

```
 coeus · opus · task 17 running · 6.1k in 0.4k out · $0.04            ● healthy
 ─────────────────────────────────────────────────────────────────────────────
 ▎ Post a tweet about the DigiByte anniversary. Use the product notes
 ▎ and keep it under 280 characters.

   Where I stand: the notes are read, drafting next.
   ▸ read memory/product.md · 2,100 characters · r3
   ▸ task · plan set, done list set
   Draft: "Nine years ago today DigiByte …"  (236 characters)
   ▸ browser_open x.com/compose/post · Compose post

 ┌ Ask me first ───────────────────────────────────────────────────────────┐
 │ browser_click e7 "Post"                                                │
 │ Posts to the DigiByte account:                                         │
 │ "Nine years ago today DigiByte …"                                      │
 │                                                                        │
 │ [a] approve once   [A] always this session   [r] reject with a reason  │
 └────────────────────────────────────────────────────────────────────────┘

 ─────────────────────────────────────────────────────────────────────────────
 › _
 waiting for you · 86 rounds, 51 min left      Enter send · Ctrl+J newline · Esc stop
```

From top to bottom:

- **Header, one row.** The wordmark `coeus`, the model alias, the task state, the cost of this session so far (tokens in and out, and money when the provider reports it), and a health dot on the right: filled and accent when the program answered its health check in the last ten seconds, hollow and dim otherwise, error-colored when disconnected. Everything dim except the task state, which is accent while a task runs and normal otherwise.
- **A rule.** One thin line, dim.
- **The transcript.** Scrolls. The newest content is at the bottom and the view sticks to the bottom until the person scrolls up, then a small `▼ more` marker appears in the status strip until they return. Four kinds of block:
  - A **person's message**, with a two-column accent bar on the left (`▎`), wrapped, in the terminal's normal foreground.
  - The **agent's reply**, streamed word by word, plain, indented two columns, with no bar. Markdown is rendered lightly: bold, code spans, fenced code in a dim frame, lists with a dash. No headings larger than the text.
  - A **tool line**, one per tool call, dim, prefixed with `▸`: the tool name, its main argument, and a short result summary with the result id once it returns (`▸ read memory/product.md · 2,100 characters · r3`). While running, the line ends with a dim spinner glyph. Tool lines never show the result text; `/tasks 17` and `read r3` do that on purpose.
  - A **card**, for the four things that need the person: a preview (title `Ask me first`), a question (title `Question`), a handoff (title `Your turn in the browser`, with the screenshot inline where the terminal can draw pictures, else its file path), and an error (title `Something went wrong`, in the error color, with what to do). A card is a single-line box in the dim color with the title in the top rule; the preview card's title is accent. The answers are listed on the last line inside the card and are single keys while the card is the focus.
- **A rule.**
- **The input box.** Starts one row, grows to five as the person types, then scrolls inside. The prompt glyph is `›` in accent. Enter sends, Ctrl+J inserts a newline, Up recalls the last message, Esc while a task runs sends stop, Ctrl+C twice quits. Typing `/` at the start opens the command palette: a dim list above the input of every registered command with its one-line help, filtered as the person types, Tab or Enter to complete.
- **The status strip, one row.** Left: the state in plain words (`idle`, `thinking`, `using read`, `waiting for you`, `paused`, `disconnected, reconnecting`), then the budget line during a task. Right: three key hints that change with the state. When a spinner is due, it sits at the far left of this row, never in the transcript.

## The masked prompt

When the program asks for a secret, the input box switches to secret mode: the prompt glyph becomes `🔒` where the terminal has it and `*` where it does not, the title of the request is shown dim above the box (`API key for anthropic`), every typed character is drawn as `•`, paste works, Enter sends, Esc cancels. Nothing typed in secret mode goes into the transcript, the history, or the log. The test types a secret and asserts the whole frame contains only bullets.

## Pictures

Screenshots from a handoff or `/screen` are drawn inline when the terminal supports the kitty graphics protocol or the iTerm2 inline image protocol (detected from the environment and a query, never assumed), scaled to the transcript width. Otherwise the card shows the file path and the words "open this file to see the page", and the path is selectable.

## Colors

Set with adaptive colors so they read on light and dark backgrounds: accent is a calm blue (`#3B82F6` on dark, `#1D4ED8` on light), dim is a gray with enough contrast to read (`#9CA3AF` on dark, `#6B7280` on light), error is a muted red (`#F87171` dark, `#B91C1C` light). Success has no color of its own; a finished task's report is a normal reply, and the done-check line in it is bold. `NO_COLOR` turns every color off and the frame must still make sense: the accent bar, the `▸`, the rules, and the card borders carry the structure without color.

## Motion

- Deltas are appended to the reply block as they arrive, coalesced every thirty milliseconds so the terminal is not repainted per word.
- The spinner is a four-frame dot cycle (`·  `, ` · `, `  ·`, ` · `) at eight frames a second, drawn only in the status strip.
- Resize re-wraps the transcript and redraws once; the view keeps its position relative to the bottom. No clearing of the screen, no flash.
- A new card scrolls into view and takes focus; the input box is dimmed until the card is answered or dismissed, and the status strip says `waiting for you`.

## What the tests check

- The golden first frame at eighty by twenty-four before any socket message: header with `coeus · connecting`, the rule, an empty transcript, the rule, the input box, and the strip saying `connecting`.
- The spinner appears at five hundred milliseconds on the fake clock and is still there at three seconds even when the reply arrived at one second.
- An approval card answered with `a`, `A`, and `r` sends the right socket message and the transcript records the answer in one dim line.
- The masked prompt frame contains only bullets.
- Resizing from eighty to sixty and back produces the same transcript text with different wrapping and no leftover characters.
- `NO_COLOR` renders the same structure.

## What the person judges in the trial

Whether the first frame is immediate, whether streaming looks smooth, whether the preview card is obvious and the three answers are discoverable, whether the masked prompt is trustworthy, and whether anything flickers on resize. Anything confusing, slow, or ugly becomes a brief.
