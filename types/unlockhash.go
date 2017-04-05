package types

import (
	"bytes"

	"github.com/NebulousLabs/Sia/crypto"
)

const (
	// UnlockHashChecksumSize is the size of the checksum used to verify
	// human-readable addresses. It is not a crypytographically secure
	// checksum, it's merely intended to prevent typos. 6 is chosen because it
	// brings the total size of the address to 38 bytes, leaving 2 bytes for
	// potential version additions in the future.
	UnlockHashChecksumSize = 6
)

// An UnlockHash is a specially constructed hash of the UnlockConditions type.
// "Locked" values can be unlocked by providing the UnlockConditions that hash
// to a given UnlockHash. See SpendConditions.UnlockHash for details on how the
// UnlockHash is constructed.
type UnlockHash crypto.Hash

type UnlockHashSlice []UnlockHash

// Len implements the Len method of sort.Interface.
func (uhs UnlockHashSlice) Len() int {
	return len(uhs)
}

// Less implements the Less method of sort.Interface.
func (uhs UnlockHashSlice) Less(i, j int) bool {
	return bytes.Compare(uhs[i][:], uhs[j][:]) < 0
}

// Swap implements the Swap method of sort.Interface.
func (uhs UnlockHashSlice) Swap(i, j int) {
	uhs[i], uhs[j] = uhs[j], uhs[i]
}
