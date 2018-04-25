// TODO currently the cached trees are not persisted and we build them at
// startup. For petabytes of data this might take a long time.

package proto

import (
	"bufio"
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

// merkleRootsPerCache is the number of merkle roots in a cached subTree of
// merkleRootsCacheHeight height.
const merkleRootsPerCache = 1 << merkleRootsCacheHeight

type (
	// merkleRoots is a helper struct that makes it easier to add/insert/remove
	// merkleRoots within a SafeContract.
	// Modifying the merkleRoots is not ACID. This means that the SafeContract
	// has to make sure it uses the WAL correctly to guarantee ACID updates to
	// the underlying file.
	merkleRoots struct {
		// cachedSubTrees are cached trees that can be used to more efficiently
		// compute the merkle root of a contract.
		cachedSubTrees []*cachedSubTree
		// uncachedRoots contains the sector roots that are not part of a
		// cached subTree. The uncachedRoots slice should never get longer than
		// 2^merkleRootsCacheHeight since that would simply result in a new
		// cached subTree in cachedSubTrees.
		uncachedRoots []crypto.Hash

		// file is the file of the safe contract that contains the roots. This
		// file is usually shared with the SafeContract which means multiple
		// threads could potentially write to the same file. That's why the
		// SafeContract should never modify the file beyond the
		// contractHeaderSize and the merkleRoots should never modify data
		// before that. Both should also use WriteAt and ReadAt instead of
		// Write and Read.
		file *os.File
		// numMerkleRoots is the number of merkle roots in file.
		numMerkleRoots int
	}

	// cachedSubTree is a cached subTree of a merkle tree. A height of 0 means
	// that the sum is the hash of a leaf. A subTree of height 1 means sum is
	// the root of 2 leaves. A subTree of height 2 contains the root of 4
	// leaves and so on.
	cachedSubTree struct {
		height int         // height of the subTree
		sum    crypto.Hash // root of the subTree
	}
)

// loadExistingMerkleRoots reads creates a merkleRoots object from existing
// merkle roots.
func loadExistingMerkleRoots(file *os.File) (*merkleRoots, error) {
	mr := &merkleRoots{
		file: file,
	}
	// Get the number of roots stored in the file.
	var err error
	mr.numMerkleRoots, err = mr.lenFromFile()
	if err != nil {
		return nil, err
	}
	// Seek to the first root's offset.
	if _, err = file.Seek(fileOffsetFromRootIndex(0), io.SeekStart); err != nil {
		return nil, err
	}
	// Read the roots from the file without reading all of them at once.
	r := bufio.NewReader(file)
	for i := 0; i < mr.numMerkleRoots; i++ {
		var root crypto.Hash
		if _, err = io.ReadFull(r, root[:]); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		// Append the root to the uncachedRoots
		mr.appendRoot(root)
	}
	return mr, nil
}

// newCachedSubTree creates a cachedSubTree from exactly
// 2^merkleRootsCacheHeight roots.
func newCachedSubTree(roots []crypto.Hash) *cachedSubTree {
	// Sanity check the input length.
	if len(roots) != merkleRootsPerCache {
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

// fileOffsetFromRootIndex calculates the offset of the merkle root at index i from
// the beginning of the contract file.
func fileOffsetFromRootIndex(i int) int64 {
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
	// If we don't have any uncached roots we need to delete the last cached
	// tree and add its elements to the uncached roots.
	if len(mr.uncachedRoots) == 0 {
		mr.cachedSubTrees = mr.cachedSubTrees[:len(mr.cachedSubTrees)-1]
		rootIndex := len(mr.cachedSubTrees) * merkleRootsPerCache
		roots, err := mr.merkleRootsFromIndex(rootIndex, mr.numMerkleRoots)
		if err != nil {
			return errors.AddContext(err, "failed to read cached tree's roots")
		}
		mr.uncachedRoots = append(mr.uncachedRoots, roots...)
	}
	// Swap the root at index i with the last root in mr.uncachedRoots.
	_, err := mr.file.WriteAt(mr.uncachedRoots[len(mr.uncachedRoots)-1][:], fileOffsetFromRootIndex(i))
	if err != nil {
		return errors.AddContext(err, "failed to swap root to delete with last one")
	}
	// If the deleted root was not cached we swap the roots in memory too.
	// Otherwise we rebuild the cachedSubTree.
	if index, cached := mr.isIndexCached(i); !cached {
		mr.uncachedRoots[index] = mr.uncachedRoots[len(mr.uncachedRoots)-1]
	} else {
		err = mr.rebuildCachedTree(index)
	}
	if err != nil {
		return errors.AddContext(err, "failed to rebuild cached tree")
	}
	// Now that the element we want to delete is the last root we can simply
	// delete it by calling mr.deleteLastRoot.
	return mr.deleteLastRoot()
}

// deleteLastRoot deletes the last sector root of the contract.
func (mr *merkleRoots) deleteLastRoot() error {
	// Decrease the numMerkleRoots counter.
	mr.numMerkleRoots--
	// Truncate file to avoid interpreting trailing data as valid.
	if err := mr.file.Truncate(fileOffsetFromRootIndex(mr.numMerkleRoots)); err != nil {
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
	rootIndex := len(mr.cachedSubTrees) * merkleRootsPerCache
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
	if index == mr.numMerkleRoots {
		return mr.push(root)
	}
	// Replaced the root on disk.
	_, err := mr.file.WriteAt(root[:], fileOffsetFromRootIndex(index))
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
	if i/merkleRootsPerCache == len(mr.cachedSubTrees) {
		// Root is not cached. Return the false and the position in
		// mr.uncachedRoots
		return i - len(mr.cachedSubTrees)*merkleRootsPerCache, false
	}
	return i / merkleRootsPerCache, true
}

// lenFromFile returns the number of merkle roots by computing it from the
// filesize.
func (mr *merkleRoots) lenFromFile() (int, error) {
	stat, err := mr.file.Stat()
	if err != nil {
		return 0, err
	}
	size := stat.Size()
	// If we haven't written a single root yet we just return 0.
	if size < contractHeaderSize {
		return 0, nil
	}

	// Sanity check contract file length.
	if (size-contractHeaderSize)%crypto.HashSize != 0 {
		return 0, errors.New("contract file has unexpected length and might be corrupted")
	}
	return int((size - contractHeaderSize) / crypto.HashSize), nil
}

// len returns the number of merkle roots. It should always return the same
// number as lenFromFile.
func (mr *merkleRoots) len() int {
	return mr.numMerkleRoots
}

// appendRoot appends a root to the in-memory structure of the merkleRoots. If
// the length of the uncachedRoots grows too large they will be compressed into
// a cachedSubTree.
func (mr *merkleRoots) appendRoot(root crypto.Hash) {
	mr.uncachedRoots = append(mr.uncachedRoots, root)
	if len(mr.uncachedRoots) == merkleRootsPerCache {
		mr.cachedSubTrees = append(mr.cachedSubTrees, newCachedSubTree(mr.uncachedRoots))
		mr.uncachedRoots = mr.uncachedRoots[:0]
	}
}

// push appends a merkle root to the end of the contract. If the number of
// uncached merkle roots grows too big we cache them in a new subTree.
func (mr *merkleRoots) push(root crypto.Hash) error {
	// Sanity check the number of uncached roots before adding a new one.
	if len(mr.uncachedRoots) == merkleRootsPerCache {
		build.Critical("the number of uncachedRoots is too big. They should've been cached by now")
	}
	// Calculate the root offset within the file and write it to disk.
	rootOffset := fileOffsetFromRootIndex(mr.len())
	if _, err := mr.file.WriteAt(root[:], rootOffset); err != nil {
		return err
	}
	// Add the root to the unached roots.
	mr.appendRoot(root)

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
	merkleRoots := make([]crypto.Hash, 0, to-from)
	if _, err := mr.file.Seek(fileOffsetFromRootIndex(from), io.SeekStart); err != nil {
		return merkleRoots, err
	}
	r := bufio.NewReader(mr.file)
	for i := from; to-i > 0; i++ {
		var root crypto.Hash
		if _, err := io.ReadFull(r, root[:]); err == io.EOF {
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
	rootIndex := index * merkleRootsPerCache
	// Read all the roots necessary for creating the cached tree.
	roots, err := mr.merkleRootsFromIndex(rootIndex, rootIndex+(1<<merkleRootsCacheHeight))
	if err != nil {
		return errors.AddContext(err, "failed to read sectors for rebuilding cached tree")
	}
	// Replace the old cached tree.
	mr.cachedSubTrees[index] = newCachedSubTree(roots)
	return nil
}
