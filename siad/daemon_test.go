package main

import (
	"testing"
)

// TestUnitPreprocessConfig probes the 'preprocessConfig' function.
func TestUnitPreprocessConfig(t *testing.T) {
	testVals := struct {
		inputs          [][]string
		expectedOutputs [][]string
	}{
		inputs: [][]string{
			[]string{"9981", "9982"},
			[]string{":9981", ":9982"},
			[]string{":9981", "9982"},
		},
		expectedOutputs: [][]string{
			[]string{":9981", ":9982"},
			[]string{":9981", ":9982"},
			[]string{":9981", ":9982"},
		},
	}
	for i := range testVals.inputs {
		config.Siad.RPCaddr = testVals.inputs[i][0]
		config.Siad.HostAddr = testVals.inputs[i][1]
		preprocessConfig()
		if config.Siad.RPCaddr != testVals.expectedOutputs[i][0] {
			t.Error("preprocessing failure at check", i, 0)
		}
		if config.Siad.HostAddr != testVals.expectedOutputs[i][1] {
			t.Error("preprocessing failure at check", i, 1)
		}
	}
}
