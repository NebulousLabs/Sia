package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/profile"
	mnemonics "github.com/NebulousLabs/entropy-mnemonics"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

// passwordPrompt securely reads a password from stdin.
func passwordPrompt(prompt string) (string, error) {
	fmt.Print(prompt)
	pw, err := terminal.ReadPassword(0)
	fmt.Println()
	return string(pw), err
}

// verifyAPISecurity checks that the security values are consistent with a
// sane, secure system.
func verifyAPISecurity(config Config) error {
	// Make sure that only the loopback address is allowed unless the
	// --disable-api-security flag has been used.
	if !config.Siad.AllowAPIBind {
		addr := modules.NetAddress(config.Siad.APIaddr)
		if !addr.IsLoopback() {
			if addr.Host() == "" {
				return fmt.Errorf("a blank host will listen on all interfaces, did you mean localhost:%v?\nyou must pass --disable-api-security to bind Siad to a non-localhost address", addr.Port())
			}
			return errors.New("you must pass --disable-api-security to bind Siad to a non-localhost address")
		}
		return nil
	}

	// If the --disable-api-security flag is used, enforce that
	// --authenticate-api must also be used.
	if config.Siad.AllowAPIBind && !config.Siad.AuthenticateAPI {
		return errors.New("cannot use --disable-api-security without setting an api password")
	}
	return nil
}

// processNetAddr adds a ':' to a bare integer, so that it is a proper port
// number.
func processNetAddr(addr string) string {
	_, err := strconv.Atoi(addr)
	if err == nil {
		return ":" + addr
	}
	return addr
}

// processModules makes the modules string lowercase to make checking if a
// module in the string easier, and returns an error if the string contains an
// invalid module character.
func processModules(modules string) (string, error) {
	modules = strings.ToLower(modules)
	validModules := "cghmrtwe"
	invalidModules := modules
	for _, m := range validModules {
		invalidModules = strings.Replace(invalidModules, string(m), "", 1)
	}
	if len(invalidModules) > 0 {
		return "", errors.New("Unable to parse --modules flag, unrecognized or duplicate modules: " + invalidModules)
	}
	return modules, nil
}

// processProfileFlags checks that the flags given for profiling are valid.
func processProfileFlags(profile string) (string, error) {
	profile = strings.ToLower(profile)
	validProfiles := "cmt"

	invalidProfiles := profile
	for _, p := range validProfiles {
		invalidProfiles = strings.Replace(invalidProfiles, string(p), "", 1)
	}
	if len(invalidProfiles) > 0 {
		return "", errors.New("Unable to parse --profile flags, unrecognized or duplicate flags: " + invalidProfiles)
	}
	return profile, nil
}

// processConfig checks the configuration values and performs cleanup on
// incorrect-but-allowed values.
func processConfig(config Config) (Config, error) {
	var err1, err2 error
	config.Siad.APIaddr = processNetAddr(config.Siad.APIaddr)
	config.Siad.RPCaddr = processNetAddr(config.Siad.RPCaddr)
	config.Siad.HostAddr = processNetAddr(config.Siad.HostAddr)
	config.Siad.Modules, err1 = processModules(config.Siad.Modules)
	config.Siad.Profile, err2 = processProfileFlags(config.Siad.Profile)
	err3 := verifyAPISecurity(config)
	err := build.JoinErrors([]error{err1, err2, err3}, ", and ")
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

// unlockWallet is called on siad startup and attempts to automatically
// unlock the wallet with the given password string.
func unlockWallet(w modules.Wallet, password string) error {
	var validKeys []crypto.TwofishKey
	dicts := []mnemonics.DictionaryID{"english", "german", "japanese"}
	for _, dict := range dicts {
		seed, err := modules.StringToSeed(password, dict)
		if err != nil {
			continue
		}
		validKeys = append(validKeys, crypto.TwofishKey(crypto.HashObject(seed)))
	}
	validKeys = append(validKeys, crypto.TwofishKey(crypto.HashObject(password)))
	for _, key := range validKeys {
		if err := w.Unlock(key); err == nil {
			return nil
		}
	}
	return modules.ErrBadEncryptionKey
}

// startDaemon uses the config parameters to initialize Sia modules and start
// siad.
func startDaemon(config Config) (err error) {
	if config.Siad.AuthenticateAPI {
		password := os.Getenv("SIA_API_PASSWORD")
		if password != "" {
			fmt.Println("Using SIA_API_PASSWORD environment variable")
			config.APIPassword = password
		} else {
			// Prompt user for API password.
			config.APIPassword, err = passwordPrompt("Enter API password: ")
			if err != nil {
				return err
			}
			if config.APIPassword == "" {
				return errors.New("password cannot be blank")
			}
		}
	}

	// Print the Siad Version
	fmt.Println("Sia Daemon v" + build.Version)
	// Print a startup message.
	fmt.Println("Loading...")
	loadStart := time.Now()

	srv, err := NewServer(config)
	if err != nil {
		return err
	}
	go srv.Serve()
	err = srv.loadModules()
	if err != nil {
		return err
	}

	// stop the server if a kill signal is caught
	errChan := make(chan error)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill)
	go func() {
		<-sigChan
		fmt.Println("\rCaught stop signal, quitting...")
		errChan <- srv.Close()
	}()

	// Print a 'startup complete' message.
	startupTime := time.Since(loadStart)
	fmt.Println("Finished loading in", startupTime.Seconds(), "seconds")

	err = <-errChan
	if err != nil {
		build.Critical(err)
	}

	return nil
}

// startDaemonCmd is a passthrough function for startDaemon.
func startDaemonCmd(cmd *cobra.Command, _ []string) {
	var profileCPU, profileMem, profileTrace bool

	profileCPU = strings.Contains(globalConfig.Siad.Profile, "c")
	profileMem = strings.Contains(globalConfig.Siad.Profile, "m")
	profileTrace = strings.Contains(globalConfig.Siad.Profile, "t")

	if build.DEBUG {
		profileCPU = true
		profileMem = true
	}

	if profileCPU || profileMem || profileTrace {
		go profile.StartContinuousProfile(globalConfig.Siad.ProfileDir, profileCPU, profileMem, profileTrace)
	}

	// Start siad. startDaemon will only return when it is shutting down.
	err := startDaemon(globalConfig)
	if err != nil {
		die(err)
	}

	// Daemon seems to have closed cleanly. Print a 'closed' mesasge.
	fmt.Println("Shutdown complete.")
}
