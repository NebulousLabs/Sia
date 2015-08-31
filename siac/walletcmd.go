package main

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/bgentry/speakeasy"
	"github.com/spf13/cobra"

	"github.com/NebulousLabs/Sia/api"
)

// coinUnits converts a siacoin amount to base units.
func coinUnits(amount string) (string, error) {
	units := []string{"pS", "nS", "uS", "mS", "SC", "KS", "MS", "GS", "TS"}
	for i, unit := range units {
		if strings.HasSuffix(amount, unit) {
			// scan into big.Rat
			r, ok := new(big.Rat).SetString(strings.TrimSuffix(amount, unit))
			if !ok {
				return "", errors.New("malformed amount")
			}
			// convert units
			exp := 24 + 3*(int64(i)-4)
			mag := new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)
			r.Mul(r, new(big.Rat).SetInt(mag))
			// r must be an integer at this point
			if !r.IsInt() {
				return "", errors.New("non-integer number of hastings")
			}
			return r.RatString(), nil
		}
	}
	// check for hastings separately
	if strings.HasSuffix(amount, "H") {
		return strings.TrimSuffix(amount, "H"), nil
	}

	return "", errors.New("amount is missing units; run 'wallet --help' for a list of units")
}

var (
	walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "Perform wallet actions",
		Long: `Generate a new address, send coins to another wallet, or view info about the wallet.

Units:
The smallest unit of siacoins is the hasting. One siacoin is 10^24 hastings. Other supported units are:
  pS (pico,  10^-12 SC)
  nS (nano,  10^-9 SC)
  uS (micro, 10^-6 SC)
  mS (milli, 10^-3 SC)
  SC
  KS (kilo, 10^3 SC)
  MS (mega, 10^6 SC)
  GS (giga, 10^9 SC)
  TS (tera, 10^12 SC)`,
		Run: wrap(walletstatuscmd),
	}

	walletAddressCmd = &cobra.Command{
		Use:   "address",
		Short: "Get a new wallet address",
		Long:  "Generate a new wallet address.",
		Run:   wrap(walletaddresscmd),
	}

	walletAddseedCmd = &cobra.Command{
		Use:   `addseed`,
		Short: "Add a seed to the wallet",
		Long:  "Uses the given password to create a new wallet with that as the primary seed",
		Run:   wrap(walletaddseedcmd),
	}

	walletInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize and encrypt a new wallet",
		Long: `Generate a new wallet from a seed string, and encrypt it.
The seed string, which is also the encryption password, will be returned.`,
		Run: wrap(walletinitcmd),
	}

	walletLoad033xCmd = &cobra.Command{
		Use:   "load033x [filepath]",
		Short: "Load a v0.3.3.x wallet",
		Long:  "Load a v0.3.3.x wallet into the current wallet",
		Run:   wrap(walletload033xcmd),
	}

	walletLockCmd = &cobra.Command{
		Use:   "lock",
		Short: "Lock the wallet",
		Long:  "Lock the wallet, preventing further use",
		Run:   wrap(walletlockcmd),
	}

	walletSeedsCmd = &cobra.Command{
		Use:   "seeds",
		Short: "Retrieve information about your seeds",
		Long:  "Retrieves the current seed, how many addresses are remaining, and the rest of your seeds from the wallet",
		Run:   wrap(walletseedscmd),
	}

	walletSendCmd = &cobra.Command{
		Use:   "send [amount] [dest]",
		Short: "Send coins to another wallet",
		Long: `Send coins to another wallet. 'dest' must be a 76-byte hexadecimal address.
'amount' can be specified in units, e.g. 1.23KS. Run 'wallet --help' for a list of units.
If no unit is supplied, hastings will be assumed.

A miner fee of 10 SC is levied on all transactions.`,
		Run: wrap(walletsendcmd),
	}

	walletSiafundsSendCmd = &cobra.Command{
		Use:   "send [amount] [dest] [keyfiles]",
		Short: "Send siafunds",
		Long: `Send siafunds to an address, and transfer their siacoins to the wallet.
Run 'wallet send --help' to see a list of available units.`,
		Run: walletsiafundssendcmd, // see function docstring
	}

	walletStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "View wallet status",
		Long:  "View wallet status, including the current balance and number of addresses.",
		Run:   wrap(walletstatuscmd),
	}

	walletUnlockCmd = &cobra.Command{
		Use:   `unlock`,
		Short: "Unlock the wallet",
		Long:  "Decrypt and load the wallet into memory",
		Run:   wrap(walletunlockcmd),
	}
)

// TODO: this should be defined outside of siac
type walletAddr struct {
	Address string
}

func walletaddresscmd() {
	addr := new(walletAddr)
	err := getAPI("/wallet/address", addr)
	if err != nil {
		fmt.Println("Could not generate new address:", err)
		return
	}
	fmt.Printf("Created new address: %s\n", addr.Address)
}

// walletaddseedcmd adds a seed to the wallet's list of seeds
func walletaddseedcmd() {
	password, err := speakeasy.Ask("Wallet password: ")
	if err != nil {
		fmt.Println("Reading password failed")
		return
	}
	seed, err := speakeasy.Ask("New Seed: ")
	if err != nil {
		fmt.Println("Reading seed failed")
		return
	}
	qs := fmt.Sprintf("encryptionpassword=%s&seed=%s&dictionary=%s", password, seed, "english")
	err = post("/wallet/seeds", qs)
	if err != nil {
		fmt.Println("Could not add seed:", err)
		return
	}
	fmt.Println("Added Key")
}

// walletinitcmd encrypts the wallet with the given password
func walletinitcmd() {
	var er api.WalletEncryptPOST
	qs := fmt.Sprintf("dictionary=%s", "english")
	if initPassword {
		password, err := speakeasy.Ask("Wallet password: ")
		if err != nil {
			fmt.Println("Reading password failed")
			return
		}
		qs += fmt.Sprintf("&encryptionpassword=%s", password)
	}
	err := postResp("/wallet/encrypt", qs, &er)
	if err != nil {
		fmt.Println("Error when encrypting wallet:", err)
		return
	}
	fmt.Printf("Seed is:\n %s\n\n", er.PrimarySeed)
	if initPassword {
		fmt.Printf("Wallet encrypted with given password\n")
	} else {
		fmt.Printf("Wallet encrypted with password: %s\n", er.PrimarySeed)
	}
}

// walletlockcmd locks the wallet
func walletlockcmd() {
	err := post("/wallet/lock", "")
	if err != nil {
		fmt.Println("Could not lock wallet:", err)
	}
}

// walletseedcmd returns the current seed {
func walletseedscmd() {
	var seedInfo api.WalletSeedsGET
	err := getAPI("/wallet/seeds", &seedInfo)
	if err != nil {
		fmt.Println("Error retrieving the current seed:", err)
		return
	}
	fmt.Printf("Primary Seed: %s\n"+
		"Addresses Remaining %d\n"+
		"All Seeds:\n", seedInfo.PrimarySeed, seedInfo.AddressesRemaining)
	for _, seed := range seedInfo.AllSeeds {
		fmt.Println(seed)
	}
}

// walletload033xcmd loads a v0.3.3.x wallet into the current wallet.
func walletload033xcmd(filepath string) {
	password, err := speakeasy.Ask("Wallet password: ")
	if err != nil {
		fmt.Println("Reading password failed")
		return
	}
	qs := fmt.Sprintf("filepath=%s&encryptionpassword=%s", filepath, password)
	err = post("/wallet/load/033x", qs)
	if err != nil {
		fmt.Println("loading error:", err)
		return
	}
	fmt.Println("Wallet loading successful.")
}

func walletsendcmd(amount, dest string) {
	adjAmount, err := coinUnits(amount)
	if err != nil {
		fmt.Println("Could not parse amount:", err)
		return
	}
	err = post("/wallet/siacoins", fmt.Sprintf("amount=%s&destination=%s", adjAmount, dest))
	if err != nil {
		fmt.Println("Could not send:", err)
		return
	}
	fmt.Printf("Sent %s hastings to %s\n", adjAmount, dest)
}

// special because list of keyfiles is variadic
func walletsiafundssendcmd(cmd *cobra.Command, args []string) {
	/*
			if len(args) < 3 {
				cmd.Usage()
				return
			}
			amount, dest, keyfiles := args[0], args[1], args[2:]
			for i := range keyfiles {
				keyfiles[i] = abs(keyfiles[i])
			}
			qs := fmt.Sprintf("amount=%s&destination=%s&keyfiles=%s", amount, dest, strings.Join(keyfiles, ","))

			err := post("/wallet/siafunds/send", qs)
			if err != nil {
				fmt.Println("Could not send siafunds:", err)
				return
			}
			fmt.Printf("Sent %s siafunds to %s\n", amount, dest)
		}

		func walletsiafundstrackcmd(keyfile string) {
			err := post("/wallet/siafunds/watchsiagaddress", "keyfile="+abs(keyfile))
			if err != nil {
				fmt.Println("Could not track siafunds:", err)
				return
			}
			fmt.Printf(`Added %s to tracked siafunds.

		You must restart siad to update your siafund balance.
		Do not delete the original keyfile.
		`, keyfile)
	*/
}

// walletstatuscmd retrieves and displays information about the wallet
func walletstatuscmd() {
	status := new(api.WalletGET)
	err := getAPI("/wallet", status)
	if err != nil {
		fmt.Println("Could not get wallet status:", err)
		return
	}
	encStatus := "Unencrypted"
	if status.Encrypted {
		encStatus = "Encrypted"
	}
	lockStatus := "Locked"
	if status.Unlocked {
		lockStatus = "Unlocked"
	}
	// divide by 1e24 to get SC
	r := new(big.Rat).SetFrac(status.ConfirmedSiacoinBalance.Big(), new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil))
	sc, _ := r.Float64()
	unconfirmedBalance := status.ConfirmedSiacoinBalance.Add(status.UnconfirmedIncomingSiacoins).Sub(status.UnconfirmedOutgoingSiacoins)
	unconfirmedDifference := new(big.Int).Sub(unconfirmedBalance.Big(), status.ConfirmedSiacoinBalance.Big())
	r = new(big.Rat).SetFrac(unconfirmedDifference, new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil))
	usc, _ := r.Float64()
	fmt.Printf(`Wallet status:
%s, %s
Confirmed Balance:   %.2f SC
Unconfirmed Delta:  %+.2f SC
Exact:               %v H
Siafunds:            %v SF
Siafund Claims:      %v SC
`, encStatus, lockStatus, sc, usc, status.ConfirmedSiacoinBalance, status.SiafundBalance, status.SiacoinClaimBalance)
}

// walletunlockcmd unlocks a saved wallet
func walletunlockcmd() {
	password, err := speakeasy.Ask("Wallet password: ")
	if err != nil {
		fmt.Println("Reading password failed")
		return
	}
	qs := fmt.Sprintf("encryptionpassword=%s&dictonary=%s", password, "english")
	err = post("/wallet/unlock", qs)
	if err != nil {
		fmt.Println("Could not unlock wallet:", err)
		return
	}
	fmt.Println("Wallet unlocked")
}
