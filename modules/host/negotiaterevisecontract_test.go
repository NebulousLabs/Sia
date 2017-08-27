package host

import (
	"testing"
)

var (
	inputs map[string][]byte
)

func init() {
	inputs = make(map[string][]byte)
	inputs["all zeros"] = make([]byte, 100)
	inputs["uniform"] = make([]byte, 256)
	for i := 0; i < 256; i++ {
		inputs["uniform"][i] = byte(i)
	}
	inputs["almost uniform"] = make([]byte, 256)
	for i := 0; i < 256-10; i++ {
		inputs["almost uniform"][i] = byte(i)
	}
}

func TestShannonEntropy(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		low, high float64
	}{
		{
			name: "all zeros",
			low:  0.0,
			high: 0.00000001,
		},
		{
			name: "uniform",
			low:  0.9999999,
			high: 1.0,
		},
		{
			name: "almost uniform",
			low:  0.98,
			high: 0.99,
		},
	}
	for _, ts := range testCases {
		got := shannonEntropy(inputs[ts.name])
		if got < ts.low || got > ts.high {
			t.Errorf("shannonEntropy(%s): %#v, want between %#v and %#v", ts.name, got, ts.low, ts.high)
		}
	}
}

func TestRandomnessTest(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		input     []byte
		wantError bool
	}{
		{
			name:      "all zeros",
			wantError: true,
		},
		{
			name:      "uniform",
			wantError: false,
		},
		{
			name:      "almost uniform",
			wantError: true,
		},
	}
	for _, ts := range testCases {
		err := randomnessTest(inputs[ts.name])
		if ts.wantError && err == nil {
			t.Errorf("shannonEntropy(%s): want error", ts.name)
		} else if !ts.wantError && err != nil {
			t.Errorf("shannonEntropy(%s): got error %v", ts.name, err)
		}
	}
}
