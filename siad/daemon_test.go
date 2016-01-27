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

// TestUnitProcessModules tests that processModules correctly processes modules
// passed to the -M / --modules flag.
func TestUnitProcessModules(t *testing.T) {
	// Test valid modules.
	testVals := []struct {
		in  string
		out string
	}{
		{"cghmrtwe", "cghmrtwe"},
		{"CGHMRTWE", "cghmrtwe"},
		{"c", "c"},
		{"g", "g"},
		{"h", "h"},
		{"m", "m"},
		{"r", "r"},
		{"t", "t"},
		{"w", "w"},
		{"e", "e"},
		{"C", "c"},
		{"G", "g"},
		{"H", "h"},
		{"M", "m"},
		{"R", "r"},
		{"T", "t"},
		{"W", "w"},
		{"E", "e"},
	}
	for _, testVal := range testVals {
		out, err := processModules(testVal.in)
		if err != nil {
			t.Error("processModules failed with error:", err)
		}
		if out != testVal.out {
			t.Errorf("processModules returned incorrect modules: expected %s, got %s\n", testVal.out, out)
		}
	}

	// Test invalid modules.
	invalidModules := []string{"abdfijklnopqsuvxyz", "cghmrtwez", "cz", "z"}
	for _, invalidModule := range invalidModules {
		_, err := processModules(invalidModule)
		if err == nil {
			t.Error("processModules didn't error on invalid module:", invalidModule)
		}
	}
}

// TestUnitProcessConfig probes the 'processConfig' function.
func TestUnitProcessConfig(t *testing.T) {
	// Test valid configs.
	testVals := struct {
		inputs          [][]string
		expectedOutputs [][]string
	}{
		inputs: [][]string{
			[]string{"9980", "9981", "9982", "cghmrtwe"},
			[]string{":9980", ":9981", ":9982", "CGHMRTWE"},
		},
		expectedOutputs: [][]string{
			[]string{":9980", ":9981", ":9982", "cghmrtwe"},
			[]string{":9980", ":9981", ":9982", "cghmrtwe"},
		},
	}
	var config Config
	for i := range testVals.inputs {
		config.Siad.APIaddr = testVals.inputs[i][0]
		config.Siad.RPCaddr = testVals.inputs[i][1]
		config.Siad.HostAddr = testVals.inputs[i][2]
		config, err := processConfig(config)
		if err != nil {
			t.Error("processConfig failed with error:", err)
		}
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

	// Test invalid configs.
	invalidModule := "z"
	config.Siad.Modules = invalidModule
	_, err := processConfig(config)
	if err == nil {
		t.Error("processModules didn't error on invalid module:", invalidModule)
	}
}
