package hostdb

/*
import (
	"errors"
	"fmt"

	"github.com/NebulousLabs/Sia/crypto"
)

// repeatCheckHelper recursively goes through nodes in the host map and adds
// them to the repeat maps.
func (hn *hostNode) repeatCheckHelper(ipMap, pkMap map[string]struct{}) error {
	ipStr := string(hn.hostEntry.NetAddress)
	pkStr := crypto.HashObject(hn.hostEntry.PublicKey).String()
	_, exists := ipMap[ipStr]
	if exists && hn.taken {
		return errors.New("found a duplicate ip address in the hostdb: " + ipStr)
	}
	_, exists = pkMap[pkStr]
	if exists && hn.taken {
		return errors.New("found a duplicate pubkey in the hostdb: " + ipStr + " " + pkStr)
	}
	if hn.taken {
		ipMap[ipStr] = struct{}{}
		pkMap[pkStr] = struct{}{}
	}

	if hn.left != nil {
		err := hn.left.repeatCheckHelper(ipMap, pkMap)
		if err != nil {
			return err
		}
	}
	if hn.right != nil {
		err := hn.right.repeatCheckHelper(ipMap, pkMap)
		if err != nil {
			return err
		}
	}
	return nil
}

// repeatCheck will return an error if there are multiple hosts in the host
// tree with the same IP address or same public key.
func repeatCheck(hn *hostNode) error {
	if hn == nil {
		return nil
	}

	ipMap := make(map[string]struct{})
	pkMap := make(map[string]struct{})
	err := hn.repeatCheckHelper(ipMap, pkMap)
	if err != nil {
		hn.String()
		return err
	}
	return nil
}

// Print recursively prints out the structure of a host tree.
func (hn *hostNode) Print() {
	if hn == nil {
		fmt.Println("EMPTY TREE")
		return
	}
	fmt.Println("Node Head:", hn.hostEntry.NetAddress, hn.count)

	if hn.left != nil {
		fmt.Println("Node Left")
		hn.left.Print()
	} else {
		fmt.Println("No Left")
	}
	if hn.right != nil {
		fmt.Println("Node Right")
		hn.right.Print()
	} else {
		fmt.Println("No Right")
	}
}
*/
