package main

import (
	"errors"
	"testing"
)

func TestIsMissingMailboxError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"Mailbox does not exist", true},
		{"folder NOT EXIST", true},
		{"Unknown Mailbox foo", true},
		{"unknown folder", true},
		{"permission denied", false},
	}
	for _, tc := range cases {
		if got := isMissingMailboxError(errors.New(tc.msg)); got != tc.want {
			t.Fatalf("%q got=%v want=%v", tc.msg, got, tc.want)
		}
	}
}

func TestIsMailboxExistsError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"Mailbox already exists", true},
		{"mailbox exists", true},
		{"folder exists", true},
		{"does not exist", false},
	}
	for _, tc := range cases {
		if got := isMailboxExistsError(errors.New(tc.msg)); got != tc.want {
			t.Fatalf("%q got=%v want=%v", tc.msg, got, tc.want)
		}
	}
}

func TestMessageHandlingErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := messageHandlingError{err: inner}
	if err.Error() != "inner" {
		t.Fatal(err.Error())
	}
	if !errors.Is(err, inner) {
		t.Fatal("unwrap")
	}
}

func TestReconnectDelayText(t *testing.T) {
	c := &fetchConfig{ReconnectDelay: 0}
	if c.reconnectDelayText() != "1s" {
		t.Fatalf("min delay=%s", c.reconnectDelayText())
	}
	c.ReconnectDelay = 5
	if c.reconnectDelayText() != "5s" {
		t.Fatalf("delay=%s", c.reconnectDelayText())
	}
}
