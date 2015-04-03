package hostdb

import (
	"errors"
	"sync"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/consensus"
	"github.com/NebulousLabs/Sia/types"
)

var (
	ErrNilState            = errors.New("hostdb can't use nil State")
	ErrMissingGenesisBlock = errors.New("state doesn't have a genesis block")
)

// The HostDB is a database of potential hosts. It assigns a weight to each
// host based on their hosting parameters, and then can select hosts at random
// for uploading files.
type HostDB struct {
	state       *consensus.State
	gateway     modules.Gateway
	recentBlock types.BlockID

	hostTree    *hostNode
	activeHosts map[string]*hostNode
	allHosts    map[modules.NetAddress]*modules.HostEntry

	mu sync.RWMutex
}

// New returns an empty HostDatabase.
func New(s *consensus.State, g modules.Gateway) (hdb *HostDB, err error) {
	if s == nil {
		err = ErrNilState
		return
	}

	genesisBlock, exists := s.BlockAtHeight(0)
	if !exists {
		if build.DEBUG {
			panic(ErrMissingGenesisBlock)
		}
		err = ErrMissingGenesisBlock
		return
	}

	hdb = &HostDB{
		state:       s,
		gateway:     g,
		recentBlock: genesisBlock.ID(),
		activeHosts: make(map[string]*hostNode),
		allHosts:    make(map[modules.NetAddress]*modules.HostEntry),
	}

	go hdb.threadedConsensusListen()

	return
}
