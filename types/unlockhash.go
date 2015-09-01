package types

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/NebulousLabs/Sia/crypto"
)

// unlockhash.go contains the unlockhash alias along with usability methods
// such as String and an implementation of sort.Interface.

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

// MarshalJSON is implemented on the unlock hash to always produce a hex string
// upon marshalling.
func (uh UnlockHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(uh.String())
}

// UnmarshalJSON is implemented on the unlock hash to recover an unlock hash
// that has been encoded to a hex string.
func (uh *UnlockHash) UnmarshalJSON(b []byte) error {
	// Check the length of b.
	if len(b) != crypto.HashSize*2+UnlockHashChecksumSize*2+2 && len(b) != crypto.HashSize*2+2 {
		return ErrUnlockHashWrongLen
	}
	return uh.LoadString(string(b[1 : len(b)-1]))
}

// String returns the hex representation of the unlock hash as a string - this
// includes a checksum.
func (uh UnlockHash) String() string {
	uhChecksum := crypto.HashObject(uh)
	return fmt.Sprintf("%x%x", uh[:], uhChecksum[:UnlockHashChecksumSize])
}

// LoadString loads a hex representation (including checksum) of an unlock hash
// into an unlock hash object. An error is returned if the string is invalid or
// fails the checksum.
func (uh *UnlockHash) LoadString(strUH string) error {
	// Check the length of strUH.
	if len(strUH) != crypto.HashSize*2+UnlockHashChecksumSize*2 && len(strUH) != crypto.HashSize*2 {
		return ErrUnlockHashWrongLen
	}

	// Decode the unlock hash.
	var byteUnlockHash []byte
	var checksum []byte
	_, err := fmt.Sscanf(strUH[:crypto.HashSize*2], "%x", &byteUnlockHash)
	if err != nil {
		return err
	}
	// Decode the checksum, if provided.
	if len(strUH) == crypto.HashSize*2+UnlockHashChecksumSize*2 {
		_, err = fmt.Sscanf(strUH[crypto.HashSize*2:], "%x", &checksum)
		if err != nil {
			return err
		}

		// Verify the checksum - leave uh as-is unless the checksum is valid.
		expectedChecksum := crypto.HashBytes(byteUnlockHash)
		if !bytes.Equal(expectedChecksum[:UnlockHashChecksumSize], checksum) {
			return ErrInvalidUnlockHashChecksum
		}
	}
	copy(uh[:], byteUnlockHash[:])

	return nil
}

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
