package browser

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/JaredTate/coeus/internal/contract"
)

// ScreenCommand builds the "/screen" command, which sends a picture of the
// browser as it stands through the channel it was typed on. The orchestrator
// registers it in serve.go.
func ScreenCommand(browser *Browser) contract.Command {
	return contract.Command{
		Name: "screen",
		Help: "Sends you a picture of the browser as it is now, with its buttons and boxes numbered.",
		Run: func(ctx context.Context, _ string, where contract.CommandContext) (string, error) {
			if where.Channel == nil {
				return "", errors.New("the screen command has nowhere to send the picture, so run it in the terminal or over Signal")
			}
			return browser.sendPicture(ctx, where.Channel)
		},
	}
}

// sendPicture photographs the page and sends it through one channel, and says
// what was sent.
func (browser *Browser) sendPicture(ctx context.Context, channel contract.Channel) (string, error) {
	picture, err := browser.Screenshot(ctx)
	if err != nil {
		return "", fmt.Errorf("the browser could not be photographed: %w", err)
	}
	pictureFile, err := savePicture(picture.PNGBase64)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(pictureFile) }()

	page := browser.CurrentAddress()
	caption := fmt.Sprintf("The browser is on %s.\n%s", page, markList(picture.Marks))
	if err := channel.SendFile(ctx, pictureFile, caption); err != nil {
		return "", fmt.Errorf("the picture of the browser would not send: %w", err)
	}
	return fmt.Sprintf("Sent you a picture of the browser on %s, with %d numbered marks on it.", page, len(picture.Marks)), nil
}
