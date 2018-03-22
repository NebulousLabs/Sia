package proto

import (
	"os"
	"path"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/errors"
	"github.com/NebulousLabs/fastrand"
)

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
	merkleRoots := newMerkleRoots(file)
	for i := 0; i < 200; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Load the existing file using LoadExistingMerkleRoots
	merkleRoots2, err := loadExistingMerkleRoots(file)
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
	merkleRoots := newMerkleRoots(file)
	for i := 0; i < 200; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Replace the last root with a new hash. It shouldn't be cached and
	// therefore no cached tree needs to be updated.
	newHash := crypto.Hash{}
	copy(newHash[:], fastrand.Bytes(crypto.HashSize)[:])
	if err := merkleRoots.insert(merkleRoots.len()-1, newHash); err != nil {
		t.Fatal("failed to insert root", err)
	}
	// Check if the second-to-last root matches the new hash.
	roots, err := merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get roots from disk", err)
	}
	if roots[len(roots)-1] != newHash {
		t.Fatal("root wasn't updated correctly on disk")
	}

	// Replace the first root with the new hash. It should be cached and
	// therefore the first cached tree should also change.
	if err := merkleRoots.insert(0, newHash); err != nil {
		t.Fatal("failed to insert root", err)
	}
	// Check if the second-to-last root matches the new hash.
	roots, err = merkleRoots.merkleRoots()
	if err != nil {
		t.Fatal("failed to get roots from disk", err)
	}
	if roots[0] != newHash {
		t.Fatal("root wasn't updated correctly on disk")
	}
	if merkleRoots.cachedSubTrees[0].sum != newCachedSubTree(roots[:1<<merkleRootsCacheHeight]).sum {
		t.Fatal("cachedSubtree doesn't have expected sum")
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
	numMerkleRoots := (1 << merkleRootsCacheHeight) + 1
	merkleRoots := newMerkleRoots(file)
	for i := 0; i < numMerkleRoots; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Delete the last sector root. This should call deleteLastRoot internally.
	if err := merkleRoots.delete(numMerkleRoots - 1); err != nil {
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
	if err := merkleRoots.delete(numMerkleRoots - 1); err != nil {
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
	if len(merkleRoots.uncachedRoots) != (1<<merkleRootsCacheHeight)-1 {
		t.Fatal("expected 2^merkleRootsCacheHeight - 1 uncached roots but was",
			len(merkleRoots.uncachedRoots))
	}
	// There should be 0 cached roots.
	if len(merkleRoots.cachedSubTrees) != 0 {
		t.Fatal("expected 0 cached roots but was", len(merkleRoots.cachedSubTrees))
	}
}

// TestDeleteLastRoot tests the deleteLastRoot method by creating many roots
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
	merkleRoots := newMerkleRoots(file)
	for i := 0; i < numMerkleRoots; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	for merkleRoots.numMerkleRoots > 0 {
		// Randomly choose a root to delete.
		deleteIndex := fastrand.Intn(merkleRoots.numMerkleRoots)

		// Get some metrics to be able to check if delete was working as expected.
		numRoots := merkleRoots.numMerkleRoots
		numCached := len(merkleRoots.cachedSubTrees)
		numUncached := len(merkleRoots.uncachedRoots)
		cachedIndex, cached := merkleRoots.isIndexCached(deleteIndex)

		if err := merkleRoots.delete(deleteIndex); err != nil {
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
			subTreeLen := int(1 << merkleRootsCacheHeight)
			from := cachedIndex * (1 << merkleRootsCacheHeight)
			roots, err := merkleRoots.merkleRootsFromIndex(from, from+subTreeLen)
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
	merkleRoots := newMerkleRoots(file)
	for i := 0; i < numMerkleRoots; i++ {
		hash := crypto.Hash{}
		copy(hash[:], fastrand.Bytes(crypto.HashSize)[:])
		merkleRoots.push(hash)
	}

	// Randomly insert or delete elements.
	for i := 0; i < numMerkleRoots; i++ {
		operation := fastrand.Intn(2)

		// Delete
		if operation == 0 {
			index := fastrand.Intn(merkleRoots.numMerkleRoots)
			if err := merkleRoots.delete(index); err != nil {
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
