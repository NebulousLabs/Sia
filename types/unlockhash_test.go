package types

import (
	"encoding/json"
	"sort"
	"testing"
)

// TestUnlockHashJSONMarshalling checks that when an unlock hash is marshalled
// and unmarshalled using JSON, the result is what is expected.
func TestUnlockHashJSONMarshalling(t *testing.T) {
	// Create an unlock hash.
	uc := UnlockConditions{
		Timelock:           5,
		SignaturesRequired: 3,
	}
	uh := uc.UnlockHash()

	// Marshal the unlock hash.
	marUH, err := json.Marshal(uh)
	if err != nil {
		t.Fatal(err)
	}

	// Unmarshal the unlock hash and compare to the original.
	var umarUH UnlockHash
	err = json.Unmarshal(marUH, &umarUH)
	if err != nil {
		t.Fatal(err)
	}
	if umarUH != uh {
		t.Error("Marshalled and unmarshalled unlock hash are not equivalent")
	}

	// Corrupt the checksum.
	marUH[36]++
	err = umarUH.UnmarshalJSON(marUH)
	if err != ErrInvalidUnlockHashChecksum {
		t.Error("expecting an invalid checksum:", err)
	}
	marUH[36]--

	// Try an input that's not correct hex.
	marUH[7] += 100
	err = umarUH.UnmarshalJSON(marUH)
	if err == nil {
		t.Error("Expecting error after corrupting input")
	}
	marUH[7] -= 100

	// Try an input of the wrong length.
	err = (&umarUH).UnmarshalJSON(marUH[2:])
	if err != ErrUnlockHashWrongLen {
		t.Error("Got wrong error:", err)
	}
}

// TestUnlockHashStringMarshalling checks that when an unlock hash is
// marshalled and unmarshalled using String and LoadString, the result is what
// is expected.
func TestUnlockHashStringMarshalling(t *testing.T) {
	// Create an unlock hash.
	uc := UnlockConditions{
		Timelock:           2,
		SignaturesRequired: 7,
	}
	uh := uc.UnlockHash()

	// Marshal the unlock hash.
	marUH := uh.String()

	// Unmarshal the unlock hash and compare to the original.
	var umarUH UnlockHash
	err := umarUH.LoadString(marUH)
	if err != nil {
		t.Fatal(err)
	}
	if umarUH != uh {
		t.Error("Marshalled and unmarshalled unlock hash are not equivalent")
	}

	// Corrupt the checksum.
	byteMarUH := []byte(marUH)
	byteMarUH[36]++
	err = umarUH.LoadString(string(byteMarUH))
	if err != ErrInvalidUnlockHashChecksum {
		t.Error("expecting an invalid checksum:", err)
	}
	byteMarUH[36]--

	// Try an input that's not correct hex.
	byteMarUH[7] += 100
	err = umarUH.LoadString(string(byteMarUH))
	if err == nil {
		t.Error("Expecting error after corrupting input")
	}
	byteMarUH[7] -= 100

	// Try an input of the wrong length.
	err = umarUH.LoadString(string(byteMarUH[2:]))
	if err != ErrUnlockHashWrongLen {
		t.Error("Got wrong error:", err)
	}
}

// TestUnlockHashSliceSorting checks that the sort method correctly sorts
// unlock hash slices.
func TestUnlockHashSliceSorting(t *testing.T) {
	// To test that byte-order is done correctly, a semi-random second byte is
	// used that is equal to the first byte * 23 % 7
	uhs := UnlockHashSlice{
		UnlockHash{4, 1},
		UnlockHash{0, 0},
		UnlockHash{2, 4},
		UnlockHash{3, 6},
		UnlockHash{1, 2},
	}
	sort.Sort(uhs)
	for i := byte(0); i < 5; i++ {
		if uhs[i] != (UnlockHash{i, (i * 23) % 7}) {
			t.Error("sorting failed on index", i, uhs[i])
		}
	}
}
