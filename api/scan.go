package api

import (
	"errors"
	"math/big"
	"strconv"

	"github.com/NebulousLabs/Sia/types"
)

// scanAmount scans a types.Currency from a string.
func scanAmount(amount string) (types.Currency, bool) {
	// use SetString manually to ensure that amount does not contain
	// multiple values, which would confuse fmt.Scan
	i, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return types.Currency{}, ok
	}
	return types.NewCurrency(i), true
}

// scanAddres scans a types.UnlockHash from a string.
func scanAddress(addrStr string) (addr types.UnlockHash, err error) {
	err = addr.LoadString(addrStr)
	return
}

// scanBlockHeight scans a block height from a string.
func scanBlockHeight(bhStr string) (types.BlockHeight, error) {
	bhInt, err := strconv.Atoi(bhStr)
	if err != nil {
		return 0, err
	}
	if bhInt < 0 {
		return 0, errors.New("negative block height not allowed")
	}
	return types.BlockHeight(bhInt), nil
}
