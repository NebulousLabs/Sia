package main

import (
	"testing"
)

// TestUnitProcessNetAddr probes the 'processNetAddr' function.
func TestUnitProcessNetAddr(t *testing.T) {
	testVals := struct {
		inputs          []string
		expectedOutputs []string
	}{
		inputs:          []string{"9980", ":9980", "localhost:9980", "test.com:9980", "192.168.14.92:9980"},
		expectedOutputs: []string{":9980", ":9980", "localhost:9980", "test.com:9980", "192.168.14.92:9980"},
	}
	for i, input := range testVals.inputs {
		output := processNetAddr(input)
		if output != testVals.expectedOutputs[i] {
			t.Error("unexpected result", i)
		}
	}
}

// TestUnitProcessConfig probes the 'processConfig' function.
func TestUnitProcessConfig(t *testing.T) {
	testVals := struct {
		inputs          [][]string
		expectedOutputs [][]string
	}{
		inputs: [][]string{
			[]string{"9980", "9981", "9982"},
			[]string{":9980", ":9981", ":9982"},
		},
		expectedOutputs: [][]string{
			[]string{":9980", ":9981", ":9982"},
			[]string{":9980", ":9981", ":9982"},
		},
	}
	var config Config
	for i := range testVals.inputs {
		config.Siad.APIaddr = testVals.inputs[i][0]
		config.Siad.RPCaddr = testVals.inputs[i][1]
		config.Siad.HostAddr = testVals.inputs[i][2]
		config = processConfig(config)
		if config.Siad.APIaddr != testVals.expectedOutputs[i][0] {
			t.Error("processing failure at check", i, 0)
		}
		if config.Siad.RPCaddr != testVals.expectedOutputs[i][1] {
			t.Error("processing failure at check", i, 1)
		}
		if config.Siad.HostAddr != testVals.expectedOutputs[i][2] {
			t.Error("processing failure at check", i, 2)
		}
	}
}
