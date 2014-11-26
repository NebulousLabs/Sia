package siad

import (
	"testing"
	"time"
)

func TestToggleMining(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	e, err := CreateEnvironment()
	if err != nil {
		t.Fatal(err)
	}

	if e.state.Height() != 0 {
		t.Error("unexpected genesis height:", e.state.Height())
	}

	e.ToggleMining()
	time.Sleep(1 * time.Second)
	e.ToggleMining()
	newHeight := e.state.Height()
	if newHeight == 0 {
		t.Error("height did not increase after mining for a second")
	}
	time.Sleep(1 * time.Second)
	if e.state.Height() != newHeight {
		t.Error("height still increasing after disabling mining...")
	}
}
