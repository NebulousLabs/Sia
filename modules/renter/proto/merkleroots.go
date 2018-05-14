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

// delete deletes the sector root at a certain index by replacing it with the
// last root and truncates the file to truncateSize after that. This ensures
// that the operation is indempotent.
func (mr *merkleRoots) delete(i int, lastRoot crypto.Hash, truncateSize int64) error {
	// Swap the element at index i with the lastRoot. This might actually
	// increase mr.numMerkleRoots since there is a chance that i points to an
	// index after the end of the file. That's why the insert is executed first
	// before truncating the file or decreasing the numMerkleRoots field.
	if err := mr.insert(i, lastRoot); err != nil {
		return errors.AddContext(err, "failed to swap deleted root with newRoot")
	}
	// Truncate the file to truncateSize.
	if err := mr.rootsFile.Truncate(truncateSize); err != nil {
		return errors.AddContext(err, "failed to truncate file")
	}
	// Adjust the numMerkleRoots field. If the number of roots didn't change we
	// are done.
	rootsBefore := mr.numMerkleRoots
	mr.numMerkleRoots = int(truncateSize / crypto.HashSize)
	if rootsBefore == mr.numMerkleRoots {
		return nil
	}
	// Sanity check the number of roots.
	if rootsBefore != mr.numMerkleRoots+1 {
		build.Critical("a delete should never delete more than one root at once")
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
	// If the index does point to an offset beyond the end of the file we fill
	// in the blanks with empty merkle roots. This usually just means that the
	// machine crashed during the recovery process and that the next few
	// updates are probably going to be delete operations that take care of the
	// blank roots.
	for index > mr.numMerkleRoots {
		if err := mr.push(crypto.Hash{}); err != nil {
			return errors.AddContext(err, "failed to extend roots")
		}
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

// prepareDelete is a helper function that returns the lastRoot and trunateSize
// arguments for a certain index to call delete with.
func (mr *merkleRoots) prepareDelete(index int) (lastRoot crypto.Hash, truncateSize int64, err error) {
	roots, err := mr.merkleRootsFromIndexFromDisk(mr.numMerkleRoots-1, mr.numMerkleRoots)
	if err != nil {
		return crypto.Hash{}, 0, errors.AddContext(err, "failed to get last root")
	}
	if len(roots) != 1 {
		return crypto.Hash{}, 0, fmt.Errorf("expected exactly 1 root but got %v", len(roots))
	}
	return roots[0], int64((mr.numMerkleRoots - 1) * crypto.HashSize), nil
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
