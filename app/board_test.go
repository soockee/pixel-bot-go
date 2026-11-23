package app

import (
	"errors"
	"testing"
)

func TestBoardOwnerRefusesASecondUser(t *testing.T) {
	var o boardOwner
	o.mu.Lock() // a run holds the Board
	if _, _, err := o.open("COM999"); !errors.Is(err, errBoardBusy) {
		t.Fatalf("open while held: err = %v, want errBoardBusy", err)
	}
	o.mu.Unlock()
}

func TestBoardOwnerLetsGoAfterAFailedOpen(t *testing.T) {
	var o boardOwner
	if _, _, err := o.open("COM999"); err == nil || errors.Is(err, errBoardBusy) {
		t.Fatalf("open of a missing port: err = %v, want a port error", err)
	}
	if !o.mu.TryLock() {
		t.Fatal("a failed open kept the Board taken")
	}
}
