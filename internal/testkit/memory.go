package testkit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// FakeMemory holds facts in memory and finds them by the words in them, which is
// enough for a test that only cares that the right fact came back.
type FakeMemory struct {
	guard sync.Mutex
	facts []contract.Fact
}

// NewFakeMemory returns a memory holding the facts given.
func NewFakeMemory(facts ...contract.Fact) *FakeMemory {
	return &FakeMemory{facts: facts}
}

// Search returns the facts whose text holds the query, newest first, up to the
// limit. A limit of zero or less means every match.
func (memory *FakeMemory) Search(_ context.Context, query string, limit int) ([]contract.Fact, error) {
	memory.guard.Lock()
	defer memory.guard.Unlock()

	found := []contract.Fact{}
	wanted := strings.ToLower(strings.TrimSpace(query))
	for _, fact := range memory.facts {
		if wanted == "" || strings.Contains(strings.ToLower(fact.Text), wanted) {
			found = append(found, fact)
		}
	}
	sort.SliceStable(found, func(left, right int) bool {
		return found[left].Recorded.After(found[right].Recorded)
	})
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}

// Get returns one fact by its id.
func (memory *FakeMemory) Get(_ context.Context, id string) (contract.Fact, error) {
	memory.guard.Lock()
	defer memory.guard.Unlock()
	for _, fact := range memory.facts {
		if fact.ID == id {
			return fact, nil
		}
	}
	return contract.Fact{}, fmt.Errorf("memory holds no fact with the id %q, so search for it instead", id)
}

// Save writes a batch of facts in one step, the way the real memory does.
func (memory *FakeMemory) Save(_ context.Context, facts []contract.Fact) error {
	for _, fact := range facts {
		if fact.Text == "" {
			return fmt.Errorf("the fact %q has no text, so give every fact something to say", fact.ID)
		}
	}
	memory.guard.Lock()
	defer memory.guard.Unlock()
	memory.facts = append(memory.facts, facts...)
	return nil
}

// Hint returns up to three lines for the end of the prompt, and nothing at all
// when nothing matches.
func (memory *FakeMemory) Hint(ctx context.Context, query string) ([]string, error) {
	found, err := memory.Search(ctx, query, contract.MemoryHintLines)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(found))
	for _, fact := range found {
		lines = append(lines, fact.Text)
	}
	return lines, nil
}
