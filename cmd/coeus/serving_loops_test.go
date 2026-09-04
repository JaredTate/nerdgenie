package main

import (
	"context"
	"testing"
)

func TestFeedingTheWatchdogIsQuietWithNoServiceManager(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// Started by hand there is no watchdog to feed, so this returns at once
	// rather than looping.
	running.feedTheWatchdog(context.Background())
}

func TestRunningWhatIsDueFindsNothingWhenNoJobIsDue(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	started, err := running.runWhatIsDue(context.Background())
	if err != nil {
		t.Fatalf("asking which task is due failed: %v", err)
	}
	if started {
		t.Error("a task was said to have started when no job is due")
	}
}

func TestTheDrainerAndTheJobDriverComeBackWhenTheContextIsDone(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	done, stop := context.WithCancel(context.Background())
	stop()

	// With the context already done, both loops must come back at once rather
	// than spin or block, because they all stop together with the socket.
	running.drain(done)
	running.runDueJobs(done)
}
