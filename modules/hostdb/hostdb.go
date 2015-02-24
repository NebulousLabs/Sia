package hostdb

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/consensus"
	"github.com/NebulousLabs/Sia/modules"
)

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters.
type HostDB struct {
	state       *consensus.State
	recentBlock consensus.BlockID

	hostTree      *hostNode
	activeHosts   map[string]*hostNode
	inactiveHosts map[string]*modules.HostEntry

	mu sync.RWMutex
}

// New returns an empty HostDatabase.
func New(state *consensus.State) (hdb *HostDB, err error) {
	if state == nil {
		err = errors.New("HostDB can't use nil State")
		return
	}
	hdb = &HostDB{
		state:         state,
		recentBlock:   state.CurrentBlock().ID(),
		activeHosts:   make(map[string]*hostNode),
		inactiveHosts: make(map[string]*modules.HostEntry),
	}
	return
}
