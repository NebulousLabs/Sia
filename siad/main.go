package main

import (
	"net/http"
)

func main() {
	// create environment
	e, err := CreateEnvironment(9989, true)
	if err != nil {
		println(err.Error())
		return
	}

	e.setUpHandlers()
}
