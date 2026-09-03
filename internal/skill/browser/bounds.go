package browser

import "github.com/JaredTate/coeus/internal/skill"

// The bounds on everything this package does. Every one of them counts
// something that comes from outside the program, and something from outside the
// program with no limit on it is a way to fill the disk, the model's context, or
// an afternoon.
const (
	// MaxRecordedSteps is how many steps one recording may hold. It is the cap
	// internal/skill already puts on a step list, because a recording is saved
	// as one and a recording the store would refuse is worth refusing earlier.
	MaxRecordedSteps = skill.MaxSteps
	// HealAttempts is how many failed steps one replay may ask the model about.
	// One is the whole budget: a second broken step means the page has moved
	// further than a patch to one descriptor can follow, and the procedure needs
	// recording again.
	HealAttempts = 1
	// MaxScreenshots is how many pictures one visual check may write, which is
	// also the most steps a walk may have, since every step is photographed.
	MaxScreenshots = 50
	// MaxElementsShownToTheModel is how many of a page's elements the self-heal
	// question carries. A page with thousands of them would otherwise be sent
	// whole to a model that is being asked about one.
	MaxElementsShownToTheModel = 100
	// MaxSeenRunes is how much of what was seen a report line quotes, so that a
	// page full of text cannot fill a report.
	MaxSeenRunes = 300
)
