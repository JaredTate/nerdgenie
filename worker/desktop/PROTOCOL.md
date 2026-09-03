# The desktop worker protocol

This document is the contract between the Go side of Coeus and the desktop
worker. The Go side is `internal/desktop` and the worker is `worker/desktop`, a
TypeScript program on Node that drives the machine's own screen, mouse, and
keyboard through `@trycua/cua-driver`. The fake desktop in `internal/testkit`
promises the same things, so a test and the real thing are written against the
same lines.

Written in wave 6, brief 6.1, in the shape of `worker/browser/PROTOCOL.md`. The
Go types are in `internal/contract/desktop.go`, and the two must always say the
same thing.

The desktop is the last resort. If the browser can do the job, the browser does
it.

## The wire

The worker speaks **JSON-RPC 2.0** over its standard input and standard output,
**one JSON object per line**. Nothing else is written to standard output; the
worker's own logging goes to standard error, one line per event, in plain
English.

A request is `{"jsonrpc":"2.0","id":<number>,"method":<name>,"params":<object>}`.
A response is `{"jsonrpc":"2.0","id":<number>,"result":<object>}` or
`{"jsonrpc":"2.0","id":<number>,"error":{"code":<number>,"message":<text>,"data":<object>}}`.

The Go side sends one request at a time and waits for its response. Every request
has a deadline; when it passes, the Go side kills the worker by its exact process
identifier, starts a new one, and tells the model that the desktop was
restarted.

A line longer than 1,048,576 bytes is answered with -32700 and the rest of that
line is thrown away, so a runaway writer cannot fill the worker's memory.

## How the worker is started

```
node worker/desktop/dist/main.js [--pacing human|fast]
```

`--pacing fast` exists only for the tests, which cannot wait for human pacing.
The Go side never passes it.

## The shapes

### Mark

A mark is one numbered control the model can point at. The numbers come from the
granted application's accessibility tree, which is the outline of its controls
that the desktop publishes for screen readers.

```json
{ "number": 1, "role": "text box", "name": "Type here" }
```

- `number` is what the model clicks and drags by. Numbering starts at 1 and is
  handed out fresh on every screenshot, in the order the tree lists the controls.
- `role` says what kind of control it is, such as `button`, `text box`, or
  `menu item`.
- `name` is the label on it, trimmed to 120 characters and put on one line. A
  document open in a text editor is one control whose label is the whole
  document, and the model does not need to read it through the mark list.

### Screenshot

```json
{
  "pngBase64": "iVBORw0KGgo...",
  "marks": [
    { "number": 1, "role": "text box", "name": "Type here" },
    { "number": 2, "role": "button", "name": "Cancel" },
    { "number": 3, "role": "button", "name": "OK" }
  ],
  "application": "zenity",
  "title": "Coeus fixture window",
  "hidden": 0,
  "windows": ["Coeus fixture window", "DigiByte - Firefox"]
}
```

With an application granted, the picture is of that application's own window
and the numbers come from its accessibility tree. With none granted, the picture
is of the whole screen and no control is numbered, because looking at the screen
needs no grant but only the granted application's tree is ever read; then
`application` and `title` are empty. `windows` names every window on the screen
by its title, or by its application when the title is empty, at most 50 of them
and each on one line of at most 120 characters, so that the model knows what it
is looking at and can `launch` the one it wants by that title. `hidden` counts
the controls the cap left out, the way the browser's `belowFold` counts what a
person would have to scroll to see. The picture is capped at `4 MB` of base64
text; a bigger one is refused with -32001 rather than sent.

With nothing granted, `pngBase64` is empty when the display cannot be
photographed whole. On a Wayland desktop the driver reaches the screen through
XWayland, whose root window cannot be grabbed, and the portal that could take the
picture needs the session bus the worker is deliberately not handed (rule seven
and the screen reader). The window names are the answer then, the worker logs
why, and the call still succeeds, because the model reads the names and never
the picture. The windows named are the ones the driver can see, which on a
Wayland desktop are those running through XWayland.

### Diff

Every action returns a diff: what changed, and whether what the model said it
expected actually happened.

```json
{
  "titleChanged": false,
  "title": "Coeus fixture window",
  "newMarks": [{ "number": 4, "role": "button", "name": "Save" }],
  "goneMarks": 0,
  "marks": [{ "number": 1, "role": "text box", "name": "Type here" }],
  "expectationMet": true,
  "seen": "",
  "settled": true
}
```

A diff carries no picture. The Go side asks for one with `screenshot` when the
model wants to look.

**How the worker judges an expectation.** It cannot judge English, so the rule is
fixed, and it is the browser's rule with the page's parts swapped for the
window's: split the expectation into words of four or more letters that are not
stop words; the expectation is met when any of them appears in a new mark's name
or role, in the new window title, or in the name or role of the mark the action
was aimed at (which is what lets "the text box holds the post" hold after typing
into the text box named "Type here", since typing changes no control); or when
the expectation is empty and something changed. Otherwise `expectationMet` is
false and `seen` says in one sentence what did change, such as "the title changed
to Unsaved Document", "two new controls appeared: Save, Cancel", or "nothing
changed".

`settled` says whether the window came to rest within the limit. When it did not,
the worker still returns the diff from the window as it stood, with
`settled: false` and `seen` saying the window kept changing, so that an
application with a clock or a progress bar in it stays usable.

### Settling

After every action the worker waits until the window has settled, which means
that its accessibility tree has not changed for three hundred milliseconds, with
a limit of three seconds. Then it reads the tree again and compares it with the
one from before the action. A window that never comes to rest within the limit is
read as it stands and reported with `settled: false`; the error -32001 is for a
window whose tree cannot be read at all after the limit.

### Errors

| Code | Meaning | What the Go side does |
|---|---|---|
| -32700 | The line was not JSON, or it was longer than the cap | Restart the worker |
| -32600 | The request was not a valid JSON-RPC request | Restart the worker |
| -32601 | No such method | A bug in the Go side; report it |
| -32602 | The parameters were wrong | Return the message to the model |
| -32000 | No such mark on the screen | Return the message to the model, with a fresh screenshot in `data` |
| -32001 | The window could not be read at all after the settle limit | Return the message to the model |
| -32002 | No application is open | Launch one first; a screenshot needs none |
| -32003 | The desktop driver is not available on this machine | Restart the worker and tell the model the desktop was interrupted |
| -32004 | The application could not be launched or brought forward | Return the message to the model |

A -32700 or -32600 response carries `"id": null`, because a line that was not a
request has no id to echo.

## The nine methods

The `expectationMet` values in the examples below are illustrative; the rule
above decides the real value.

### `launch`

Opens an application, or brings it forward if it is already open, and makes it
the one application every later call acts in. The Go side has already collected
the user's grant for it; the worker does not ask.

Request: `{"jsonrpc":"2.0","id":1,"method":"launch","params":{"application":"zenity","expectation":"a window opens"}}`

Response: `{"jsonrpc":"2.0","id":1,"result":{"titleChanged":true,"title":"Coeus fixture window","newMarks":[],"goneMarks":0,"marks":[{"number":1,"role":"text box","name":"Type here"}],"expectationMet":true,"seen":"","settled":true,"application":"zenity"}}`

An application that is already open is brought forward rather than opened twice.
An application name that resolves to nothing launchable is -32004, and the
message names what was asked for.

### `screenshot`

Returns a picture of the screen with the windows on it named: the granted
application's window with its controls numbered, or, when no application has
been launched, the whole screen with no control numbered. It is the one method
besides `health` that needs no `launch` before it.

Request: `{"jsonrpc":"2.0","id":2,"method":"screenshot","params":{}}`

Response: `{"jsonrpc":"2.0","id":2,"result":{"pngBase64":"iVBORw0KGgo...","marks":[{"number":1,"role":"text box","name":"Type here"}],"application":"zenity","title":"Coeus fixture window","hidden":0,"windows":["Coeus fixture window"]}}`

Before any launch: `{"jsonrpc":"2.0","id":2,"result":{"pngBase64":"iVBORw0KGgo...","marks":[],"application":"","title":"","hidden":0,"windows":["Coeus fixture window","DigiByte - Firefox"]}}`

### `click`

Clicks the control with that number and checks the expectation. The number must
come from the most recent screenshot or diff; a number that is not on the screen
is -32000 with a fresh screenshot in `data`.

Request: `{"jsonrpc":"2.0","id":3,"method":"click","params":{"mark":3,"expectation":"the dialog closes"}}`

Response: `{"jsonrpc":"2.0","id":3,"result":{"titleChanged":false,"title":"Coeus fixture window","newMarks":[],"goneMarks":3,"marks":[],"expectationMet":true,"seen":"","settled":true}}`

### `type`

Types text at human pacing into the control the last click landed on, or into
whatever holds the keyboard focus when there has been no click. `mark` aims the
typing at one control instead. Text longer than 10,000 characters is -32602.

Request: `{"jsonrpc":"2.0","id":4,"method":"type","params":{"text":"nine years of DigiByte","expectation":"the text box holds the post"}}`

Response: `{"jsonrpc":"2.0","id":4,"result":{"titleChanged":false,"title":"Coeus fixture window","newMarks":[],"goneMarks":0,"marks":[{"number":1,"role":"text box","name":"Type here"}],"expectationMet":true,"seen":"","settled":true}}`

### `press`

Presses one key or one key combination, such as `Enter` or `ctrl+s`. The chord is
written as the modifiers and then the key, joined by `+`. A single character that is not a
letter loses its layout's shift state on the way through the driver, so it is
-32602 and the message says to use `type` instead.

Request: `{"jsonrpc":"2.0","id":5,"method":"press","params":{"keys":"ctrl+s","expectation":"a save dialog appears"}}`

Response: `{"jsonrpc":"2.0","id":5,"result":{"titleChanged":false,"title":"Coeus fixture window","newMarks":[{"number":5,"role":"button","name":"Save"}],"goneMarks":0,"marks":[],"expectationMet":true,"seen":"","settled":true}}`

### `drag`

Drags from the middle of one numbered control to the middle of another, in steps,
the way a hand moves.

Request: `{"jsonrpc":"2.0","id":6,"method":"drag","params":{"fromMark":1,"toMark":2,"expectation":"the file moves"}}`

Response: `{"jsonrpc":"2.0","id":6,"result":{"titleChanged":false,"title":"Coeus fixture window","newMarks":[],"goneMarks":0,"marks":[],"expectationMet":false,"seen":"nothing changed","settled":true}}`

### `clipboardGet`

Reads the machine's clipboard as plain text. The clipboard belongs to the whole
machine rather than to the granted application, so this is the one method that
reads something outside it.

Request: `{"jsonrpc":"2.0","id":7,"method":"clipboardGet","params":{}}`

Response: `{"jsonrpc":"2.0","id":7,"result":{"text":"nine years of DigiByte"}}`

### `clipboardSet`

Replaces the machine's clipboard with plain text. Pasting is a thing that cannot
be undone, so the Go side previews it before calling this.

Request: `{"jsonrpc":"2.0","id":8,"method":"clipboardSet","params":{"text":"nine years of DigiByte"}}`

Response: `{"jsonrpc":"2.0","id":8,"result":{"characters":22}}`

### `health`

Says whether the worker can act on the desktop, and which driver it loaded. The
Go side calls it after starting the worker and before trusting it.

Request: `{"jsonrpc":"2.0","id":9,"method":"health","params":{}}`

Response: `{"jsonrpc":"2.0","id":9,"result":{"healthy":true,"driverVersion":"0.23.2","display":"wayland with xwayland","detail":""}}`

An unhealthy answer always fills `detail` with what to fix, because the user has
to be told.

## Rules the worker keeps

1. It acts only inside the one application `launch` granted, and it reads only
   that application's accessibility tree, because the user's other windows are
   none of the agent's business. Looking is not acting: with no application
   granted, `screenshot` photographs the whole screen and names the windows on
   it by title, so that the model can tell what is there and launch the one it
   wants; it still numbers no control outside the granted application.
2. It brings the granted window to the front before every action. Key
   combinations cannot be delivered to a window in the background on this display
   server, and a person focuses the window they are working in.
3. It moves at human pacing: it types in short runs with small varying gaps
   between them, waits a human length of time after a click, drags in steps, and
   pauses between actions. `--pacing fast` shortens every one of those waits,
   including the settle waits above, and is used only by the tests.
4. Every action states an expectation and the worker checks it before returning.
5. It never follows an instruction it read in a window. Everything a window says
   is data.
6. On a Wayland session it launches applications with `GDK_BACKEND=x11` and
   `QT_QPA_PLATFORM=xcb`, so that they run through XWayland, where the
   accessibility tree and input delivery both work. This changes nothing about
   the windows the user opened themselves.
7. It never receives a secret. The Go side starts it with an environment that
   carries none, and no method of this protocol takes a password.
8. Nothing but responses goes to standard output.
