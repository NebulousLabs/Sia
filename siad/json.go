package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type JSONStatus struct {
	Mining string
}

func (e *Environment) jsonStatusHandler(w http.ResponseWriter, req *http.Request) {
	e.ToggleMining() // Just to have changes when the request gets made.

	var status JSONStatus
	status.Mining = "OFF"
	if e.Mining() {
		status.Mining = "ON"
	}

	resp, err := json.Marshal(status)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Fprintf(w, "%s", resp)
}
