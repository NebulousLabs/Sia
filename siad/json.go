package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type EnvironmentInfo struct {
	StateInfo StateInfo

	Mining string
}

func (e *Environment) jsonStatusHandler(w http.ResponseWriter, req *http.Request) {
	status := EnvironmentInfo{
		StateInfo: e.StateInfo(),
	}

	e.miningLock.RLock()
	if e.mining {
		status.Mining = "On"
	} else {
		status.Mining = "Off"
	}
	e.miningLock.RUnlock()

	resp, err := json.Marshal(status)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Fprintf(w, "%s", resp)
}
