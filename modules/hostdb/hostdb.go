package hostdb

import (
	"errors"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/sync"
)

var (
	ErrNilState            = errors.New("hostdb can't use nil State")
	ErrMissingGenesisBlock = errors.New("state doesn't have a genesis block")
)

type (
	// The HostDB is a database of potential hosts. It assigns a weight to each
	// host based on their hosting parameters, and then can select hosts at random
	// for uploading files.
	HostDB struct {
		state   *consensus.State
		gateway modules.Gateway

		hostTree    *hostNode
		activeHosts map[string]*hostNode
		allHosts    map[modules.NetAddress]*modules.HostEntry

		mu *sync.RWMutex
	}
)

// New returns an empty HostDatabase.
func New(cs *consensus.State, g modules.Gateway) (hdb *HostDB, err error) {
	if cs == nil {
		err = ErrNilState
		return
	}

	hdb = &HostDB{
		state:   cs,
		gateway: g,

		activeHosts: make(map[string]*hostNode),
		allHosts:    make(map[modules.NetAddress]*modules.HostEntry),

		mu: sync.New(1*time.Second, 0),
	}

	cs.Subscribe(hdb)

	return
}
