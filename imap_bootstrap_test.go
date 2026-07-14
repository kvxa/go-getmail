package main

import "testing"

func TestShouldHandleBootstrap(t *testing.T) {
	if shouldHandleBootstrap(0) {
		t.Fatal("empty mailbox should skip bootstrap handle")
	}
	if !shouldHandleBootstrap(1) {
		t.Fatal("non-empty mailbox should handle bootstrap")
	}
	if !shouldHandleBootstrap(12) {
		t.Fatal("non-empty mailbox should handle bootstrap")
	}
}

func TestStateName(t *testing.T) {
	cases := map[fetchState]string{
		initialState:    "initial",
		connectingState: "connecting",
		connectedState:  "connected",
		watchingState:   "watching",
		handlingState:   "handling",
		shutdownState:   "shutdown",
		fetchState(99):  "unknown",
	}
	for state, want := range cases {
		if got := stateName(state); got != want {
			t.Fatalf("stateName(%d)=%q want %q", state, got, want)
		}
	}
}
