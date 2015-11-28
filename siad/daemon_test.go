package main

import (
	"testing"
)

// TestUnitProcessConfig probes the 'processConfig' function.
func TestUnitProcessConfig(t *testing.T) {
	testVals := struct {
		inputs          [][]string
		expectedOutputs [][]string
	}{
		inputs: [][]string{
			[]string{"9980", "9981", "9982"},
			[]string{":9980", ":9981", ":9982"},
			[]string{"localhost:9980", "localhost:9981", "localhost:9982"},
			[]string{"localhost:9980", ":9981", "9982"},
		},
		expectedOutputs: [][]string{
			[]string{":9980", ":9981", ":9982"},
			[]string{":9980", ":9981", ":9982"},
			[]string{"localhost:9980", "localhost:9981", "localhost:9982"},
			[]string{"localhost:9980", ":9981", ":9982"},
		},
	}
	for i := range testVals.inputs {
		config.Siad.RPCaddr = testVals.inputs[i][0]
		config.Siad.HostAddr = testVals.inputs[i][1]
		processConfig()
		if config.Siad.RPCaddr != testVals.expectedOutputs[i][0] {
			t.Error("processing failure at check", i, 0)
		}
		if config.Siad.HostAddr != testVals.expectedOutputs[i][1] {
			t.Error("processing failure at check", i, 1)
		}
	}
}
