package proto

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/fastrand"
)

// cmpRoots is a helper function that compares the in-memory file structure and
// on-disk roots of two merkleRoots objects.
func cmpRoots(m1, m2 *merkleRoots) error {
	roots1, err1 := m1.merkleRoots()
	roots2, err2 := m2.merkleRoots()
	if err1 != nil || err2 != nil {
		return errors.AddContext(errors.Compose(err1, err2), "failed to compare on-disk roots")
	}
	if len(roots1) != len(roots2) {
		return fmt.Errorf("len of roots on disk doesn't match %v != %v",
			len(roots1), len(roots2))
	}
	if len(m1.uncachedRoots) != len(m2.uncachedRoots) {
		return fmt.Errorf("len of uncachedRoots doesn't match %v != %v",
			len(m1.uncachedRoots), len(m2.uncachedRoots))
	}
	if len(m1.cachedSubTrees) != len(m2.cachedSubTrees) {
		return fmt.Errorf("len of cached subTrees doesn't match %v != %v",
			len(m1.cachedSubTrees), len(m2.cachedSubTrees))
	}
	if m1.numMerkleRoots != m2.numMerkleRoots {
		return fmt.Errorf("numMerkleRoots fields don't match %v != %v",
			m1.numMerkleRoots, m2.numMerkleRoots)
	}
	for i := 0; i < len(roots1); i++ {
		if roots1[i] != roots2[i] {
			return errors.New("on-disk roots don't match")
		}
	}
	for i := 0; i < len(m1.uncachedRoots); i++ {
		if m1.uncachedRoots[i] != m2.uncachedRoots[i] {
			return errors.New("uncached roots don't match")
		}
	}
	for i := 0; i < len(m1.cachedSubTrees); i++ {
		if !reflect.DeepEqual(m1.cachedSubTrees[i], m2.cachedSubTrees[i]) {
			return fmt.Errorf("cached trees at index %v don't match", i)
		}
	}
	return nil
}

// TestLoadExistingMerkleRoots tests if it is possible to load existing merkle
// roots from disk.
func TestLoadExistingMerkleRoots(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create a file for the test.
	dir := build.TempDir(t.Name())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := path.Join(dir, "file.dat")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create sector roots.
	rootSection := newFileSection(file, 0, -1)
	merkleRoots := newMerkleRoots(rootSection)
	for i := 0; i < 200; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Load the existing file using LoadExistingMerkleRoots
	merkleRoots2, err := loadExistingMerkleRoots(rootSection)
	if err != nil {
		t.Fatal(err)
	}
	if merkleRoots2.len() != merkleRoots.len() {
		t.Errorf("expected len %v but was %v", merkleRoots.len(), merkleRoots2.len())
	}
	// Check if they have the same roots.
	roots, err := merkleRoots.merkleRoots()
	roots2, err2 := merkleRoots2.merkleRoots()
	if errors.Compose(err, err2) != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(roots); i++ {
		if roots[i] != roots2[i] {
			t.Errorf("roots at index %v don't match", i)
		}
	}
	// Check if the cached subTrees match.
	if len(merkleRoots.cachedSubTrees) != len(merkleRoots2.cachedSubTrees) {
		t.Fatalf("expected %v cached trees but got %v",
			len(merkleRoots.cachedSubTrees), len(merkleRoots2.cachedSubTrees))
	}

	// Check if the computed roots match.
	if merkleRoots.root() != merkleRoots2.root() {
		t.Fatal("the roots don't match")
	}
}

// TestInsertMerkleRoot tests the merkleRoots' insert method.
func TestInsertMerkleRoot(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dir := build.TempDir(t.Name())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := path.Join(dir, "file.dat")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create sector roots.
	rootSection := newFileSection(file, 0, -1)
	merkleRoots := newMerkleRoots(rootSection)
	for i := 0; i < 200; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Replace the last root with a new hash. It shouldn't be cached and
	// therefore no cached tree needs to be updated.
	newHash := crypto.Hash{}
	insertIndex := merkleRoots.len() - 1
	copy(newHash[:], fastrand.Bytes(crypto.HashSize)[:])
	if err := merkleRoots.insert(insertIndex, newHash); err != nil {
		t.Fatal("failed to insert root", err)
	}
	// Insert again at the same index to make sure insert is idempotent.
	if err := merkleRoots.insert(insertIndex, newHash); err != nil {
		t.Fatal("failed to insert root", err)
	}
	// Check if the last root matches the new hash.
	roots, err := merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get roots from disk", err)
	}
	if roots[len(roots)-1] != newHash {
		t.Fatal("root wasn't updated correctly on disk")
	}
	// Reload the roots. The in-memory structure and the roots on disk should
	// still be consistent.
	loadedRoots, err := loadExistingMerkleRoots(merkleRoots.rootsFile)
	if err != nil {
		t.Fatal("failed to load existing roots", err)
	}
	if err := cmpRoots(merkleRoots, loadedRoots); err != nil {
		t.Fatal("loaded roots are inconsistent", err)
	}
	// Replace the first root with the new hash. It should be cached and
	// therefore the first cached tree should also change.
	if err := merkleRoots.insert(0, newHash); err != nil {
		t.Fatal("failed to insert root", err)
	}
	// Check if the first root matches the new hash.
	roots, err = merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get roots from disk", err)
	}
	if roots[0] != newHash {
		t.Fatal("root wasn't updated correctly on disk")
	}
	if merkleRoots.cachedSubTrees[0].sum != newCachedSubTree(roots[:merkleRootsPerCache]).sum {
		t.Fatal("cachedSubtree doesn't have expected sum")
	}
	// Reload the roots. The in-memory structure and the roots on disk should
	// still be consistent.
	loadedRoots, err = loadExistingMerkleRoots(merkleRoots.rootsFile)
	if err != nil {
		t.Fatal("failed to load existing roots", err)
	}
	if err := cmpRoots(merkleRoots, loadedRoots); err != nil {
		t.Fatal("loaded roots are inconsistent", err)
	}
}

// TestDeleteLastRoot tests the deleteLastRoot method.
func TestDeleteLastRoot(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dir := build.TempDir(t.Name())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := path.Join(dir, "file.dat")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create sector roots. We choose the number of merkle roots in a way that
	// makes the first delete remove a uncached root and the second delete has
	// to remove a cached tree.
	numMerkleRoots := merkleRootsPerCache + 1
	rootSection := newFileSection(file, 0, -1)
	merkleRoots := newMerkleRoots(rootSection)
	for i := 0; i < numMerkleRoots; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Delete the last sector root. This should call deleteLastRoot internally.
	lastRoot, truncateSize, err := merkleRoots.prepareDelete(numMerkleRoots - 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkleRoots.delete(numMerkleRoots-1, lastRoot, truncateSize); err != nil {
		t.Fatal("failed to delete last root", err)
	}
	numMerkleRoots--
	// Check if the number of roots actually decreased.
	if merkleRoots.numMerkleRoots != numMerkleRoots {
		t.Fatal("numMerkleRoots wasn't decreased")
	}
	// Check if the file was truncated.
	if roots, err := merkleRoots.merkleRoots(); err != nil {
		t.Fatal("failed to get roots from disk", err)
	} else if len(roots) != merkleRoots.numMerkleRoots {
		t.Fatal("roots on disk don't match number of roots in memory")
	}
	// There should be 0 uncached roots now.
	if len(merkleRoots.uncachedRoots) != 0 {
		t.Fatal("expected 0 uncached roots but was", len(merkleRoots.uncachedRoots))
	}
	// There should be 1 cached roots.
	if len(merkleRoots.cachedSubTrees) != 1 {
		t.Fatal("expected 1 cached root but was", len(merkleRoots.cachedSubTrees))
	}

	// Delete the last sector root again. This time a cached root should be deleted too.
	lastRoot, truncateSize, err = merkleRoots.prepareDelete(numMerkleRoots - 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkleRoots.delete(numMerkleRoots-1, lastRoot, truncateSize); err != nil {
		t.Fatal("failed to delete last root", err)
	}
	numMerkleRoots--
	// Check if the number of roots actually decreased.
	if merkleRoots.numMerkleRoots != numMerkleRoots {
		t.Fatal("numMerkleRoots wasn't decreased")
	}
	// Check if the file was truncated.
	if roots, err := merkleRoots.merkleRoots(); err != nil {
		t.Fatal("failed to get roots from disk", err)
	} else if len(roots) != merkleRoots.numMerkleRoots {
		t.Fatal("roots on disk don't match number of roots in memory")
	}
	// There should be 2^merkleRootsCacheHeight - 1 uncached roots now.
	if len(merkleRoots.uncachedRoots) != merkleRootsPerCache-1 {
		t.Fatal("expected 2^merkleRootsCacheHeight - 1 uncached roots but was",
			len(merkleRoots.uncachedRoots))
	}
	// There should be 0 cached roots.
	if len(merkleRoots.cachedSubTrees) != 0 {
		t.Fatal("expected 0 cached roots but was", len(merkleRoots.cachedSubTrees))
	}

	// Reload the roots. The in-memory structure and the roots on disk should
	// still be consistent.
	loadedRoots, err := loadExistingMerkleRoots(merkleRoots.rootsFile)
	if err != nil {
		t.Fatal("failed to load existing roots", err)
	}
	if err := cmpRoots(merkleRoots, loadedRoots); err != nil {
		t.Fatal("loaded roots are inconsistent", err)
	}
}

// TestDelete tests the deleteRoot method by creating many roots
// and deleting random indices until there are no more roots left.
func TestDelete(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dir := build.TempDir(t.Name())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := path.Join(dir, "file.dat")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create many sector roots.
	numMerkleRoots := 1000
	rootSection := newFileSection(file, 0, -1)
	merkleRoots := newMerkleRoots(rootSection)
	for i := 0; i < numMerkleRoots; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	for merkleRoots.numMerkleRoots > 0 {
		// 1% chance to reload the roots and check if they are consistent.
		if fastrand.Intn(100) == 0 {
			loadedRoots, err := loadExistingMerkleRoots(merkleRoots.rootsFile)
			if err != nil {
				t.Fatal("failed to load existing roots", err)
			}
			if err := cmpRoots(loadedRoots, merkleRoots); err != nil {
				t.Fatal(err)
			}
		}
		// Randomly choose a root to delete.
		deleteIndex := fastrand.Intn(merkleRoots.numMerkleRoots)

		// Get some metrics to be able to check if delete was working as expected.
		numRoots := merkleRoots.numMerkleRoots
		numCached := len(merkleRoots.cachedSubTrees)
		numUncached := len(merkleRoots.uncachedRoots)
		cachedIndex, cached := merkleRoots.isIndexCached(deleteIndex)

		// Call delete twice to make sure it's idempotent.
		lastRoot, truncateSize, err := merkleRoots.prepareDelete(deleteIndex)
		if err != nil {
			t.Fatal(err)
		}
		if err := merkleRoots.delete(deleteIndex, lastRoot, truncateSize); err != nil {
			t.Fatal("failed to delete random index", deleteIndex, err)
		}
		if err := merkleRoots.delete(deleteIndex, lastRoot, truncateSize); err != nil {
			t.Fatal("failed to delete random index", deleteIndex, err)
		}
		// Number of roots should have decreased.
		if merkleRoots.numMerkleRoots != numRoots-1 {
			t.Fatal("number of roots in memory should have decreased")
		}
		// Number of roots on disk should have decreased.
		if roots, err := merkleRoots.merkleRoots(); err != nil {
			t.Fatal("failed to get roots from disk")
		} else if len(roots) != numRoots-1 {
			t.Fatal("number of roots on disk should have decreased")
		}
		// If the number of uncached roots was >0 the cached roots should be
		// the same and the number of uncached roots should have decreased by 1.
		if numUncached > 0 && !(len(merkleRoots.cachedSubTrees) == numCached && len(merkleRoots.uncachedRoots) == numUncached-1) {
			t.Fatal("deletion of uncached root failed")
		}
		// If the number of uncached roots was 0, there should be 1 less cached
		// root and the uncached roots should have length
		// 2^merkleRootsCacheHeight-1.
		if numUncached == 0 && !(len(merkleRoots.cachedSubTrees) == numCached-1 && len(merkleRoots.uncachedRoots) == (1<<merkleRootsCacheHeight)-1) {
			t.Fatal("deletion of cached root failed")
		}
		// If the deleted root was cached we expect the cache to have the
		// correct, updated value.
		if cached && len(merkleRoots.cachedSubTrees) > cachedIndex {
			subTreeLen := merkleRootsPerCache
			from := cachedIndex * merkleRootsPerCache
			roots, err := merkleRoots.merkleRootsFromIndexFromDisk(from, from+subTreeLen)
			if err != nil {
				t.Fatal("failed to read roots of subTree", err)
			}
			if merkleRoots.cachedSubTrees[cachedIndex].sum != newCachedSubTree(roots).sum {
				t.Fatal("new cached root sum doesn't match expected sum")
			}
		}
	}
}

// TestMerkleRootsRandom creates a large number of merkle roots and runs random
// valid operations on them that shouldn't result in any errors.
func TestMerkleRootsRandom(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	dir := build.TempDir(t.Name())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := path.Join(dir, "file.dat")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}

	// Create many sector roots.
	numMerkleRoots := 10000
	rootSection := newFileSection(file, 0, -1)
	merkleRoots := newMerkleRoots(rootSection)
	for i := 0; i < numMerkleRoots; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Randomly insert or delete elements.
	for i := 0; i < numMerkleRoots; i++ {
		// 1% chance to reload the roots and check if they are consistent.
		if fastrand.Intn(100) == 0 {
			loadedRoots, err := loadExistingMerkleRoots(merkleRoots.rootsFile)
			if err != nil {
				t.Fatal("failed to load existing roots")
			}
			if err := cmpRoots(loadedRoots, merkleRoots); err != nil {
				t.Fatal(err)
			}
		}
		operation := fastrand.Intn(2)

		// Delete
		if operation == 0 {
			index := fastrand.Intn(merkleRoots.numMerkleRoots)
			lastRoot, truncateSize, err := merkleRoots.prepareDelete(index)
			if err != nil {
				t.Fatal(err)
			}
			if err := merkleRoots.delete(index, lastRoot, truncateSize); err != nil {
				t.Fatalf("failed to delete %v: %v", index, err)
			}
			continue
		}

		// Insert
		var hash crypto.Hash
		copy(hash[:], fastrand.Bytes(len(hash)))
		index := fastrand.Intn(merkleRoots.numMerkleRoots + 1)
		if err := merkleRoots.insert(index, hash); err != nil {
			t.Fatalf("failed to insert %v at %v: %v", hash, index, err)
		}
	}
}
