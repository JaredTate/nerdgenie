package contract

import "context"

// DesktopMark is one numbered clickable thing on a screenshot of the desktop.
type DesktopMark struct {
	// Number is the number drawn on the picture.
	Number int `json:"number"`
	// Role says what kind of control it is, such as "button" or "menu item".
	Role string `json:"role"`
	// Name is the label on it.
	Name string `json:"name"`
}

// DesktopScreenshot is a picture of the screen with its controls numbered.
type DesktopScreenshot struct {
	// PNGBase64 is the picture, encoded as base64 text.
	PNGBase64 string `json:"pngBase64"`
	// Marks lists what each number points at.
	Marks []DesktopMark `json:"marks"`
	// Application is the application the picture is of, and is empty when no
	// application is open and the picture is of the whole screen.
	Application string `json:"application"`
	// Windows names every window on the screen by its title, so that the model
	// knows what it is looking at and can launch the one it wants.
	Windows []string `json:"windows"`
}

// Desktop drives the screen, the mouse, and the keyboard through the desktop
// worker. It is the last resort: if the browser can do the job, the browser does
// it. An application must be granted once per session before it can be used, and
// anything that cannot be undone still gets a preview.
type Desktop interface {
	// Launch opens an application, or brings it forward if it is already open,
	// and checks what the model expected to happen.
	Launch(ctx context.Context, application string, expectation string) error
	// Screenshot returns the screen with its controls numbered. It needs no
	// application: with none open, no control is numbered, the windows on the
	// screen are named, and the picture is of the whole screen, or empty where
	// the display cannot be photographed whole.
	Screenshot(ctx context.Context) (DesktopScreenshot, error)
	// Click clicks the control with that number and checks what the model
	// expected to happen.
	Click(ctx context.Context, mark int, expectation string) error
	// Type types text at human pacing.
	Type(ctx context.Context, text string, expectation string) error
	// Press presses a key combination, such as "ctrl+s", and checks what the
	// model expected to happen.
	Press(ctx context.Context, keys string, expectation string) error
	// Drag drags from one numbered control to another and checks what the model
	// expected to happen.
	Drag(ctx context.Context, fromMark int, toMark int, expectation string) error
	// Clipboard reads what is on the clipboard.
	Clipboard(ctx context.Context) (string, error)
	// SetClipboard puts text on the clipboard.
	SetClipboard(ctx context.Context, text string) error
}
