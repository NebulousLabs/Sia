package proto

import (
	"fmt"
	"io"
	"os"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/errors"
)

// merkleRootsCacheHeight is the height of the subTrees in cachedSubTrees. A
// height of 7 means that 128 sector roots are covered by a single cached
// subTree.
const merkleRootsCacheHeight = 7

type (
	// merkleRoots is a helper struct that makes it easier to add/insert/remove
	// merkleRoots within a SafeContract.
	merkleRoots struct {
		// cachedSubTrees are cached trees that can be used to more efficiently
		// compute the merkle root of a contract. The cached trees are not
		// persisted and are computed after startup.
		cachedSubTrees []*cachedSubTree
		// uncachedRoots contains the sector roots that are not part of a
		// cached subTree. The uncachedRoots slice should never get longer than
		// 2^merkleRootsCacheHeight since that would simply result in a new
		// cached subTree in cachedSubTrees.
		uncachedRoots []crypto.Hash

		// file is the file of the safe contract that contains the root.
		file *os.File
		// numMerkleRoots is the number of merkle roots in file.
		numMerkleRoots int
	}

	cachedSubTree struct {
		height int
		sum    crypto.Hash
	}
)

// loadExistingMerkleRoots reads creates a merkleRoots object from existing
// merkle roots.
func loadExistingMerkleRoots(file *os.File) (mr *merkleRoots, err error) {
	mr = &merkleRoots{
		file: file,
	}
	// Get the number of roots stored in the file.
	mr.numMerkleRoots, err = mr.lenFromFile()
	if err != nil {
		return
	}
	// Seek to the first root's offset.
	if _, err = file.Seek(rootIndexToOffset(0), io.SeekStart); err != nil {
		return
	}
	// Read the roots from the file without reading all of them at once.
	for i := 0; i < mr.numMerkleRoots; i++ {
		var root crypto.Hash
		if _, err = io.ReadFull(file, root[:]); err == io.EOF {
			break
		} else if err != nil {
			return
		}

		// Append the root to the unachedRoots
		mr.uncachedRoots = append(mr.uncachedRoots, root)

		// If the uncachedRoots grew too large we add them to the cache.
		if len(mr.uncachedRoots) == (1 << merkleRootsCacheHeight) {
			st := newCachedSubTree(mr.uncachedRoots)
			mr.cachedSubTrees = append(mr.cachedSubTrees, st)
			mr.uncachedRoots = mr.uncachedRoots[:0]
		}
	}
	return mr, nil
}

// newCachedSubTree creates a cachedSubTree from exactly
// 2^merkleRootsCacheHeight roots.
func newCachedSubTree(roots []crypto.Hash) *cachedSubTree {
	// Sanity check the input length.
	if len(roots) != (1 << merkleRootsCacheHeight) {
		build.Critical("can't create a cached subTree from the provided number of roots")
	}
	return &cachedSubTree{
		height: int(merkleRootsCacheHeight + sectorHeight),
		sum:    cachedMerkleRoot(roots),
	}
}

// newMerkleRoots creates a new merkleRoots object. This doesn't load existing
// roots from file and will assume that the file doesn't contain any roots.
// Don't use this on a file that contains roots.
func newMerkleRoots(file *os.File) *merkleRoots {
	return &merkleRoots{
		file: file,
	}
}

// rootIndexToOffset calculates the offset of the merkle root at index i from
// the beginning of the contract file.
func rootIndexToOffset(i int) int64 {
	return contractHeaderSize + crypto.HashSize*int64(i)
}

// delete deletes the sector root at a certain index.
func (mr *merkleRoots) delete(i int) error {
	// Check if the index is correct
	if i >= mr.numMerkleRoots {
		build.Critical("can't delete non-existing root")
		return nil
	}
	// If i is the index of the last element we call deleteLastRoot.
	if i == mr.numMerkleRoots-1 {
		return mr.deleteLastRoot()
	}
	// - swap root at i with last root of mr.uncachedRoots. If that is not
	// possible because len(mr.uncachedRoots) == 0 we need to delete the last
	// cache and append its elements to mr.uncachedRoots before we swap.
	// - if root at i is in a cache we need to reconstruct that cache after swapping.
	// - call deleteLastRoot to get rid of the swapped element at the end of mr.u
	panic("not implemented yet")
}

// deleteLastRoot deletes the last sector root of the contract.
func (mr *merkleRoots) deleteLastRoot() error {
	// Decrease the numMerkleRoots counter.
	mr.numMerkleRoots--
	// Truncate file to avoid interpreting trailing data as valid.
	if err := mr.file.Truncate(rootIndexToOffset(mr.numMerkleRoots)); err != nil {
		return errors.AddContext(err, "failed to delete last root from file")
	}
	// If the last element is uncached we can simply remove it from the slice.
	if len(mr.uncachedRoots) > 0 {
		mr.uncachedRoots = mr.uncachedRoots[:len(mr.uncachedRoots)-1]
		return nil
	}
	// If it is not uncached we need to delete the last cached tree and load
	// its elements into mr.uncachedRoots.
	mr.cachedSubTrees = mr.cachedSubTrees[:len(mr.cachedSubTrees)-1]
	rootIndex := len(mr.cachedSubTrees) * (1 << merkleRootsCacheHeight)
	roots, err := mr.merkleRootsFromIndex(rootIndex, mr.numMerkleRoots)
	if err != nil {
		return errors.AddContext(err, "failed to read cached tree's roots")
	}
	mr.uncachedRoots = append(mr.uncachedRoots, roots...)
	return nil
}

// insert inserts a root by replacing a root at an existing index.
func (mr *merkleRoots) insert(index int, root crypto.Hash) error {
	if index > mr.numMerkleRoots {
		return errors.New("can't insert at a index greater than the number of roots")
	}
	// Replaced the root on disk.
	_, err := mr.file.WriteAt(root[:], rootIndexToOffset(index))
	if err != nil {
		return errors.AddContext(err, "failed to insert root on disk")
	}

	// Find out if the root is in mr.cachedSubTree or mr.uncachedRoots.
	i, cached := mr.isIndexCached(index)
	// If the root was not cached we can simply replace it in mr.uncachedRoots.
	if !cached {
		mr.uncachedRoots[i] = root
		return nil
	}
	// If the root was cached we need to rebuild the cache.
	if err := mr.rebuildCachedTree(i); err != nil {
		return errors.AddContext(err, "failed to rebuild cache for inserted root")
	}
	return nil
}

// isIndexCached determines if the root at index i is already cached in
// mr.cachedSubTree or if it is still in mr.uncachedRoots. It will return true
// or false and the index of the root in the corresponding data structure.
func (mr *merkleRoots) isIndexCached(i int) (int, bool) {
	if i/(1<<merkleRootsCacheHeight) == len(mr.cachedSubTrees) {
		// Root is not cached. Return the false and the position in
		// mr.uncachedRoots
		return i - len(mr.cachedSubTrees)*(1<<merkleRootsCacheHeight), false
	}
	return i / (1 << merkleRootsCacheHeight), true
}

// lenFromFile returns the number of merkle roots by computing it from the
// filesize.
func (mr *merkleRoots) lenFromFile() (int, error) {
	offset, err := mr.file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	// If we haven't written a single root yet we just return 0.
	rootStart := rootIndexToOffset(0)
	if offset < rootStart {
		return 0, nil
	}

	// Sanity check contract file length.
	if (offset-rootStart)%crypto.HashSize != 0 {
		build.Critical("contract file has unexpected length and might be corrupted.")
	}
	return int((offset - rootStart) / crypto.HashSize), nil
}

// len returns the number of merkle roots. It should always return the same
// number as lenFromFile.
func (mr *merkleRoots) len() int {
	return mr.numMerkleRoots
}

// push appends a merkle root to the end of the contract. If the number of
// uncached merkle roots grows too big we cache them in a new subTree.
func (mr *merkleRoots) push(root crypto.Hash) error {
	// Sanity check the number of uncached roots before adding a new one.
	if len(mr.uncachedRoots) == (1 << merkleRootsCacheHeight) {
		build.Critical("the number of uncachedRoots is too big. They should've been cached by now")
	}
	// Calculate the root offset within the file and write it to disk.
	rootOffset := rootIndexToOffset(mr.len())
	if _, err := mr.file.WriteAt(root[:], rootOffset); err != nil {
		return err
	}
	// Add the root to the unached roots. If uncachedRoots is big enoug we can
	// cache those roots.
	mr.uncachedRoots = append(mr.uncachedRoots, root)
	if len(mr.uncachedRoots) == (1 << merkleRootsCacheHeight) {
		mr.cachedSubTrees = append(mr.cachedSubTrees, newCachedSubTree(mr.uncachedRoots))
		mr.uncachedRoots = mr.uncachedRoots[:0]
	}

	// Increment the number of roots.
	mr.numMerkleRoots++
	return nil
}

// root returns the root of the merkle roots.
func (mr *merkleRoots) root() crypto.Hash {
	tree := crypto.NewCachedTree(sectorHeight)
	for _, st := range mr.cachedSubTrees {
		if err := tree.PushSubTree(st.height, st.sum); err != nil {
			// This should never fail.
			build.Critical(err)
		}
	}
	for _, root := range mr.uncachedRoots {
		tree.Push(root)
	}
	return tree.Root()
}

// newRoot returns the root of the merkleTree after appending the newRoot
// without actually appending it.
func (mr *merkleRoots) newRoot(newRoot crypto.Hash) crypto.Hash {
	tree := crypto.NewCachedTree(sectorHeight)
	for _, st := range mr.cachedSubTrees {
		if err := tree.PushSubTree(st.height, st.sum); err != nil {
			// This should never fail.
			build.Critical(err)
		}
	}
	for _, root := range mr.uncachedRoots {
		tree.Push(root)
	}
	// Push the new root.
	tree.Push(newRoot)
	return tree.Root()
}

// merkleRoots reads all the merkle roots from disk and returns them. This is
// not very fast and should only be used for testing purposes.
func (mr *merkleRoots) merkleRoots() (roots []crypto.Hash, err error) {
	// Get roots.
	roots, err = mr.merkleRootsFromIndex(0, mr.numMerkleRoots)
	if err != nil {
		return nil, err
	}
	// Sanity check: should have read exactly numMerkleRoots roots.
	if len(roots) != mr.numMerkleRoots {
		build.Critical(fmt.Sprintf("Number of merkle roots on disk (%v) doesn't match numMerkleRoots (%v)",
			len(roots), mr.numMerkleRoots))
	}
	return
}

// merkleRootsFrom index readds all the merkle roots in range [from;to)
func (mr *merkleRoots) merkleRootsFromIndex(from, to int) ([]crypto.Hash, error) {
	merkleRoots := make([]crypto.Hash, 0, mr.numMerkleRoots-1)
	if _, err := mr.file.Seek(rootIndexToOffset(from), io.SeekStart); err != nil {
		return merkleRoots, err
	}
	for i := from; to-i > 0; i++ {
		var root crypto.Hash
		if _, err := io.ReadFull(mr.file, root[:]); err == io.EOF {
			return nil, io.ErrUnexpectedEOF
		} else if err != nil {
			return merkleRoots, errors.AddContext(err, "failed to read root from disk")
		}
		merkleRoots = append(merkleRoots, root)
	}
	return merkleRoots, nil
}

// rebuildCachedTree rebuilds the tree in mr.cachedSubTree at index i.
func (mr *merkleRoots) rebuildCachedTree(index int) error {
	// Find the index of the first root of the cached tree on disk.
	rootIndex := index * (1 << merkleRootsCacheHeight)
	// Read all the roots necessary for creating the cached tree.
	roots, err := mr.merkleRootsFromIndex(rootIndex, rootIndex+(1<<merkleRootsCacheHeight))
	if err != nil {
		return errors.AddContext(err, "failed to read sectors for rebuilding cached tree")
	}
	// Replace the old cached tree.
	mr.cachedSubTrees[index] = newCachedSubTree(roots)
	return nil
}
