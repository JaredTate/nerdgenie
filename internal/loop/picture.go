package loop

import (
	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A picture a tool hands back rides to a model that can see, and is dropped
// with a line saying so for one that cannot. Pictures are dear: a 1024 by 768
// screenshot came to about eight hundred tokens on the local daemon on 6
// September 2026, so only the newest few are kept, and the working context
// puts them in one message at the end of the prompt rather than beside their
// results, so that a picture coming or going changes no byte above the tail.

// MaxPicturesShown is how many pictures ride in the prompt at once.
const MaxPicturesShown = workingcontext.MaxPicturesShown

// ThePictureIsNotShown says plainly what a picture is to a model with no eyes:
// the fifth game build's play-test task asked for a screenshot four times
// running and read the same window list each time, because nothing said the
// picture went nowhere.
const ThePictureIsNotShown = "the picture itself is not shown to you; you read the lines above in its place"

// ThePictureIsNoLongerShown is what the pictures message says of the older
// pictures that have left the prompt.
const ThePictureIsNoLongerShown = workingcontext.ThePictureIsNoLongerShown

// withOrWithoutThePicture decides what a tool's picture becomes: the result's
// own for a model that can see, and a line for one that cannot.
func (running *run) withOrWithoutThePicture(text string, picture string) (string, string) {
	if picture == "" {
		return text, ""
	}
	if running.theLoop.options.Vision {
		return text, picture
	}
	return text + "\n" + ThePictureIsNotShown, ""
}

// keepTheNewestPictures drops every picture but the newest MaxPicturesShown
// from the conversation the loop holds. The result's text is left as it was
// written: the prompt says in its pictures message that older pictures are no
// longer shown, so nothing in the conversation is rewritten.
func (running *run) keepTheNewestPictures() {
	kept := 0
	for at := len(running.messages) - 1; at >= 0; at-- {
		results := running.messages[at].ToolResults
		for back := len(results) - 1; back >= 0; back-- {
			if results[back].Picture == "" {
				continue
			}
			if kept < MaxPicturesShown {
				kept++
				continue
			}
			results[back].Picture = ""
		}
	}
}

// pictureOf is the picture of a tool's output, or nothing.
func pictureOf(output contract.ToolOutput) string {
	return output.Picture
}
