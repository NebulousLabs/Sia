package host

import (
	"fmt"
	"math"
	"math/rand"
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
	inputs["random"] = make([]byte, 1<<22)
	n, err := rand.New(rand.NewSource(42)).Read(inputs["random"])
	if n != len(inputs["random"]) || err != nil {
		panic(fmt.Errorf("failed to initialize 'random': n=%d err=%v", n, err))
	}
}

func TestShannonEntropy(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		want float64
	}{
		{
			name: "all zeros",
			want: 0.0,
		},
		{
			name: "uniform",
			want: 1.0,
		},
		{
			name: "almost uniform",
			want: 0.981419,
		},
		{
			name: "random",
			want: 0.999994,
		},
	}
	maxDelta := 0.000001
	for _, ts := range testCases {
		got := shannonEntropy(inputs[ts.name])
		if math.Abs(got-ts.want) > maxDelta {
			t.Errorf("shannonEntropy(%s): %#v, want %#v", ts.name, got, ts.want)
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
		{
			name:      "random",
			wantError: false,
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
