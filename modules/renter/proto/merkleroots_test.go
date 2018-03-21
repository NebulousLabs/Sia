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
