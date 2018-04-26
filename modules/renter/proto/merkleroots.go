package proto

// TODO currently the cached trees are not persisted and we build them at
// startup. For petabytes of data this might take a long time.

import (
	"fmt"
	"io"

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

		// rootsFile is the rootsFile of the safe contract that contains the roots.
		rootsFile *fileSection
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

// parseRootsFromData takes some data and splits it up into sector roots. It will return an error if the size of the data is not a multiple of crypto.HashSize.
func parseRootsFromData(b []byte) ([]crypto.Hash, error) {
	var roots []crypto.Hash
	if len(b)%crypto.HashSize != 0 {
		return roots, errors.New("roots have unexpected length and might be corrupted")
	}

	var root crypto.Hash
	for len(b) > 0 {
		copy(root[:], b[:crypto.HashSize])
		roots = append(roots, root)
		b = b[crypto.HashSize:]
	}
	return roots, nil
}

// loadExistingMerkleRoots reads creates a merkleRoots object from existing
// merkle roots.
func loadExistingMerkleRoots(file *fileSection) (*merkleRoots, error) {
	mr := &merkleRoots{
		rootsFile: file,
	}
	// Get the number of roots stored in the file.
	var err error
	mr.numMerkleRoots, err = mr.lenFromFile()
	if err != nil {
		return nil, err
	}
	// Read the roots from the file without reading all of them at once.
	readOff := int64(0)
	rootsData := make([]byte, rootsDiskLoadBulkSize)
	for {
		n, err := file.ReadAt(rootsData, readOff)
		if err == io.ErrUnexpectedEOF && n == 0 {
			break
		}
		if err == io.EOF && n == 0 {
			break
		}
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		roots, err := parseRootsFromData(rootsData[:n])
		if err != nil {
			return nil, err
		}
		mr.appendRootMemory(roots...)
		readOff += int64(n)
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
func newMerkleRoots(file *fileSection) *merkleRoots {
	return &merkleRoots{
		rootsFile: file,
	}
}

// fileOffsetFromRootIndex calculates the offset of the merkle root at index i from
// the beginning of the contract file.
func fileOffsetFromRootIndex(i int) int64 {
	return crypto.HashSize * int64(i)
}

// appendRootMemory appends a root to the in-memory structure of the merkleRoots. If
// the length of the uncachedRoots grows too large they will be compressed into
// a cachedSubTree.
func (mr *merkleRoots) appendRootMemory(roots ...crypto.Hash) {
	for _, root := range roots {
		mr.uncachedRoots = append(mr.uncachedRoots, root)
		if len(mr.uncachedRoots) == merkleRootsPerCache {
			mr.cachedSubTrees = append(mr.cachedSubTrees, newCachedSubTree(mr.uncachedRoots))
			mr.uncachedRoots = mr.uncachedRoots[:0]
		}
	}
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
		if err := mr.moveLastCachedSubTreeToUncached(); err != nil {
			return err
		}
	}
	// Swap the root at index i with the last root in mr.uncachedRoots.
	_, err := mr.rootsFile.WriteAt(mr.uncachedRoots[len(mr.uncachedRoots)-1][:], fileOffsetFromRootIndex(i))
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
	if err := mr.rootsFile.Truncate(fileOffsetFromRootIndex(mr.numMerkleRoots)); err != nil {
		return errors.AddContext(err, "failed to delete last root from file")
	}
	// If the last element is uncached we can simply remove it from the slice.
	if len(mr.uncachedRoots) > 0 {
		mr.uncachedRoots = mr.uncachedRoots[:len(mr.uncachedRoots)-1]
		return nil
	}
	// If it is not uncached we need to delete the last cached tree and load
	// its elements into mr.uncachedRoots. This should give us
	// merkleRootsPerCache-1 uncached roots since we already truncated the file
	// by 1 root.
	if err := mr.moveLastCachedSubTreeToUncached(); err != nil {
		return err
	}
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
	_, err := mr.rootsFile.WriteAt(root[:], fileOffsetFromRootIndex(index))
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
	size, err := mr.rootsFile.Size()
	if err != nil {
		return 0, err
	}

	// Sanity check contract file length.
	if size%crypto.HashSize != 0 {
		return 0, errors.New("contract file has unexpected length and might be corrupted")
	}
	return int(size / crypto.HashSize), nil
}

// len returns the number of merkle roots. It should always return the same
// number as lenFromFile.
func (mr *merkleRoots) len() int {
	return mr.numMerkleRoots
}

// moveLastCachedSubTreeToUncached deletes the last cached subTree and appends
// its elements to the uncached roots.
func (mr *merkleRoots) moveLastCachedSubTreeToUncached() error {
	mr.cachedSubTrees = mr.cachedSubTrees[:len(mr.cachedSubTrees)-1]
	rootIndex := len(mr.cachedSubTrees) * merkleRootsPerCache
	roots, err := mr.merkleRootsFromIndexFromDisk(rootIndex, mr.numMerkleRoots)
	if err != nil {
		return errors.AddContext(err, "failed to read cached tree's roots")
	}
	mr.uncachedRoots = append(mr.uncachedRoots, roots...)
	return nil
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
	if _, err := mr.rootsFile.WriteAt(root[:], rootOffset); err != nil {
		return err
	}
	// Add the root to the unached roots.
	mr.appendRootMemory(root)

	// Increment the number of roots.
	mr.numMerkleRoots++
	return nil
}

// root returns the root of the merkle roots.
func (mr *merkleRoots) root() crypto.Hash {
	tree := crypto.NewTree()
	for _, st := range mr.cachedSubTrees {
		if err := tree.PushSubTree(st.height, st.sum[:]); err != nil {
			// This should never fail.
			build.Critical(err)
		}
	}
	for _, root := range mr.uncachedRoots {
		tree.Push(root[:])
	}
	return tree.Root()
}

// checkNewRoot returns the root of the merkleTree after appending the checkNewRoot
// without actually appending it.
func (mr *merkleRoots) checkNewRoot(newRoot crypto.Hash) crypto.Hash {
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

// merkleRoots reads all the merkle roots from disk and returns them.
func (mr *merkleRoots) merkleRoots() (roots []crypto.Hash, err error) {
	// Get roots.
	roots, err = mr.merkleRootsFromIndexFromDisk(0, mr.numMerkleRoots)
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

// merkleRootsFrom index reads all the merkle roots in range [from;to)
func (mr *merkleRoots) merkleRootsFromIndexFromDisk(from, to int) ([]crypto.Hash, error) {
	merkleRoots := make([]crypto.Hash, 0, to-from)
	remainingData := fileOffsetFromRootIndex(to) - fileOffsetFromRootIndex(from)
	readOff := fileOffsetFromRootIndex(from)
	var rootsData []byte
	for remainingData > 0 {
		if remainingData > rootsDiskLoadBulkSize {
			rootsData = make([]byte, rootsDiskLoadBulkSize)
		} else {
			rootsData = make([]byte, remainingData)
		}
		n, err := mr.rootsFile.ReadAt(rootsData, readOff)
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			return nil, errors.New("merkleRootsFromIndexFromDisk failed: roots have unexpected length")
		}
		if err != nil {
			return nil, err
		}
		roots, err := parseRootsFromData(rootsData)
		if err != nil {
			return nil, err
		}
		merkleRoots = append(merkleRoots, roots...)
		readOff += int64(n)
		remainingData -= int64(n)
	}
	return merkleRoots, nil
}

// rebuildCachedTree rebuilds the tree in mr.cachedSubTree at index i.
func (mr *merkleRoots) rebuildCachedTree(index int) error {
	// Find the index of the first root of the cached tree on disk.
	rootIndex := index * merkleRootsPerCache
	// Read all the roots necessary for creating the cached tree.
	roots, err := mr.merkleRootsFromIndexFromDisk(rootIndex, rootIndex+(1<<merkleRootsCacheHeight))
	if err != nil {
		return errors.AddContext(err, "failed to read sectors for rebuilding cached tree")
	}
	// Replace the old cached tree.
	mr.cachedSubTrees[index] = newCachedSubTree(roots)
	return nil
}
