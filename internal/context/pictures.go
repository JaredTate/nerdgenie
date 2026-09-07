package context

import (
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A picture rides in one message at the very end of the prompt, replaced each
// round, and never inside the conversation. On 6 September 2026 the newest two
// pictures rode with their results and an older result was rewritten when its
// picture left, which changed bytes in the middle of the conversation and cost
// the daemon its cache from that point on eighteen times in one day. Nothing
// above the tail changes now when a picture comes or goes.

// MaxPicturesShown is how many pictures ride in the prompt at once. A 1024 by
// 768 screenshot came to about eight hundred tokens on the local daemon.
const MaxPicturesShown = 2

// ThePictureIsNoLongerShown tells the model how to see a picture that has left
// the prompt.
const ThePictureIsNoLongerShown = "older pictures are no longer shown; read the file a result names to see one again"

// picturesHeading opens the message the pictures ride in.
const picturesHeading = "## Pictures"

// withoutPictures is the conversation with every picture taken off its result,
// and the newest MaxPicturesShown pictures on their own, newest first, each
// labelled by the result it came with and carrying no call id, because it
// answers no call where it now rides.
func withoutPictures(messages []contract.Message) ([]contract.Message, []contract.ToolResult) {
	stripped := make([]contract.Message, len(messages))
	pictures := []contract.ToolResult{}
	for at := len(messages) - 1; at >= 0; at-- {
		message := messages[at]
		if len(message.ToolResults) > 0 {
			results := make([]contract.ToolResult, len(message.ToolResults))
			copy(results, message.ToolResults)
			for back := len(results) - 1; back >= 0; back-- {
				if results[back].Picture == "" {
					continue
				}
				if len(pictures) < MaxPicturesShown {
					pictures = append(pictures, contract.ToolResult{Label: results[back].Label, Picture: results[back].Picture})
				}
				results[back].Picture = ""
			}
			message.ToolResults = results
		}
		stripped[at] = message
	}
	return stripped, pictures
}

// picturesMessage is the one user message the newest pictures ride in, or
// nothing when there are none.
func picturesMessage(pictures []contract.ToolResult) []contract.Message {
	if len(pictures) == 0 {
		return nil
	}
	labels := make([]string, 0, len(pictures))
	for _, picture := range pictures {
		labels = append(labels, picture.Label)
	}
	text := picturesHeading + "\n\nthe pictures that came with " + strings.Join(labels, " and ") + ", newest first; " + ThePictureIsNoLongerShown
	return []contract.Message{{Role: contract.RoleUser, Text: text, ToolResults: pictures}}
}
