package sia

import (
	"os"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/encoding"
)

// LoadCoinAddress loads a coin address from a file and adds that address to
// the friend list using the input name. An error is returned if the name is
// already in the friend list.
func (e *Environment) LoadCoinAddress(filename string, friendName string) (err error) {
	// Open the file and read the key to a friend map.
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	// Read the contents of the file into a buffer.
	buffer := make([]byte, 32)
	bytes, err := file.Read(buffer)
	if err != nil {
		return
	}

	// Decode the bytes into an address.
	var address consensus.CoinAddress
	err = encoding.Unmarshal(buffer[:bytes], &address)
	if err != nil {
		return
	}

	// Add the address to the friends list.
	e.friends[friendName] = address

	return
}

func (e *Environment) FriendMap() (safeMap map[string]consensus.CoinAddress) {
	safeMap = make(map[string]consensus.CoinAddress)
	for key, value := range e.friends {
		safeMap[key] = value
	}
	return
}
