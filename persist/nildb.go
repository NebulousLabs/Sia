package persist

import (
	"github.com/NebulousLabs/Sia/types"
)

// NilDB is a db whose methods are no-ops. Calls to Block will return the zero
// value of types.Block. Calls to Height will return 0.
var NilDB nildb

type nildb struct{}

func (n nildb) Block(types.BlockHeight) (types.Block, error) { return types.Block{}, nil }
func (n nildb) AddBlock(types.Block) error                   { return nil }
func (n nildb) RemoveBlock() error                           { return nil }
func (n nildb) Height() (types.BlockHeight, error)           { return 0, nil }
func (n nildb) Close() error                                 { return nil }
