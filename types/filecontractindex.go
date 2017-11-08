package types

// filecontractindex.go defines the filecontractindex object.
// it is an unsigned integer index into a filecontract siacoin output.

type (
	// A FileContractIndex represents an unsigned integer index into the
	// SiacoinOutput array associated with a FileContract.
	FileContractIndex struct {
		Index uint
	}
)

var (
	FileContractIndexRenter = NewFileContractIndex(0)
	FileContractIndexHost   = NewFileContractIndex(1)
	FileContractIndexVoid   = NewFileContractIndex(2)
)

// NewFileContractIndex creates a FileContractIndex from a uint index
func NewFileContractIndex(index uint) (c FileContractIndex) {
	c.Index = index
	return
}
