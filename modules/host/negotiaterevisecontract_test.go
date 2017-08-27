package host

import (
	"testing"
)

func TestShannonEntropy(t *testing.T) {
	t.Parallel()
	uniform := make([]byte, 256)
	for i := 0; i < 256; i++ {
		uniform[i] = byte(i)
	}
	almost := make([]byte, 256)
	for i := 0; i < 256-10; i++ {
		almost[i] = byte(i)
	}
	testCases := []struct {
		name      string
		input     []byte
		low, high float64
	}{
		{
			name:  "all zeros",
			input: make([]byte, 100),
			low:   0.0,
			high:  0.00000001,
		},
		{
			name:  "uniform distrinution",
			input: uniform,
			low:   0.9999999,
			high:  1.0,
		},
		{
			name:  "almost uniform distrinution",
			input: almost,
			low:   0.98,
			high:  0.99,
		},
	}
	for _, ts := range testCases {
		got := shannonEntropy(ts.input)
		if got < ts.low || got > ts.high {
			t.Errorf("shannonEntropy(%s): %#v, want between %#v and %#v", ts.name, got, ts.low, ts.high)
		}
	}
}

func TestRandomnessTest(t *testing.T) {
	t.Parallel()
	uniform := make([]byte, 256)
	for i := 0; i < 256; i++ {
		uniform[i] = byte(i)
	}
	almost := make([]byte, 256)
	for i := 0; i < 256-10; i++ {
		almost[i] = byte(i)
	}
	testCases := []struct {
		name      string
		input     []byte
		wantError bool
	}{
		{
			name:      "all zeros",
			input:     make([]byte, 100),
			wantError: true,
		},
		{
			name:      "uniform distrinution",
			input:     uniform,
			wantError: false,
		},
		{
			name:      "almost uniform distrinution",
			input:     almost,
			wantError: true,
		},
	}
	for _, ts := range testCases {
		err := randomnessTest(ts.input)
		if ts.wantError && err == nil {
			t.Errorf("shannonEntropy(%s): want error", ts.name)
		} else if !ts.wantError && err != nil {
			t.Errorf("shannonEntropy(%s): got error %v", ts.name, err)
		}
	}
}
