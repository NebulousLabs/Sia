package main

import (
	"fmt"
)

// displayHomeHelp lists all of the options available at the home screen, with
// descriptions.
func displayHomeHelp() {
	fmt.Println(
		" h:\tHelp\n",
		"q:\tQuit\n",
		"s:\tSend coins to another wallet.\n",
	)
}

// pollHome repeatedly querys the user for input.
func pollHome(env *walletEnvironment) {
	var input string
	var err error
	for {
		fmt.Println()
		fmt.Print("(Home) Please enter a command: ")
		_, err = fmt.Scanln(&input)
		if err != nil {
			continue
		}
		fmt.Println()

		switch input {
		default:
			fmt.Println("unrecognized command")

		case "?", "h", "help":
			displayHomeHelp()

		case "q", "quit":
			return

			/*
				case "S", "save":
					saveWalletEnvironmentWalkthorugh()
			*/

		case "s", "send"
			sendCoinsWalkthrough()
		}
	}
}
