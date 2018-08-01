package proto

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/fastrand"
)

// SafeReadAt is a wrapper for ReadAt that recovers from a potential panic and
// returns it as an error.
func (f *fileSection) SafeReadAt(b []byte, off int64) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return f.ReadAt(b, off)
}

// SafeTruncate is a wrapper for Truncate that recovers from a potential panic
// and returns it as an error.
func (f *fileSection) SafeTruncate(size int64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return f.Truncate(size)
}

// SafeWriteAt is a wrapper for WriteAt that recovers from a potential panic
// and returns it as an error.
func (f *fileSection) SafeWriteAt(b []byte, off int64) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return f.WriteAt(b, off)
}

// TestFileSectionBoundariesValidReadWrites uses valid read and write
// operations on the fileSection to make sure that the data is written to and
// read from the section correctly without corrupting other sections.
func TestFileSectionBoundariesValidReadWrites(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir(t.Name())
	if err := os.MkdirAll(testDir, 0700); err != nil {
		t.Fatal(err)
	}
	testFile, err := os.Create(filepath.Join(testDir, "testfile.dat"))
	if err != nil {
		t.Fatal(err)
	}

	// Create 3 sections.
	s1Size := 100
	s2Size := 100
	s1 := newFileSection(testFile, 0, int64(s1Size))
	s2 := newFileSection(testFile, int64(s1Size), int64(s1Size+s2Size))
	s3 := newFileSection(testFile, int64(s1Size+s2Size), remainingFile)

	// Write as much data to the sections as they can fit.
	s1Data := fastrand.Bytes(s1Size)
	s2Data := fastrand.Bytes(s2Size)
	s3Data := fastrand.Bytes(s2Size) // s3 has an infinite size so we just write s2Size bytes.

	n, err := s1.SafeWriteAt(s1Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(s1Data) {
		t.Fatalf("expected %v bytes to be written instead of %v", len(s1Data), n)
	}
	n, err = s2.SafeWriteAt(s2Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(s2Data) {
		t.Fatalf("expected %v bytes to be written instead of %v", len(s2Data), n)
	}
	n, err = s3.SafeWriteAt(s3Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(s3Data) {
		t.Fatalf("expected %v bytes to be written instead of %v", len(s3Data), n)
	}

	// Read the written data from the file and check if it matches.
	readS1Data := make([]byte, len(s1Data))
	readS2Data := make([]byte, len(s2Data))
	readS3Data := make([]byte, len(s3Data))
	_, err = s1.SafeReadAt(readS1Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s2.SafeReadAt(readS2Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s3.SafeReadAt(readS3Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := testFile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	size := fi.Size()
	size1, err := s1.Size()
	if err != nil {
		t.Fatal(err)
	}
	size2, err := s2.Size()
	if err != nil {
		t.Fatal(err)
	}
	size3, err := s3.Size()
	if err != nil {
		t.Fatal(err)
	}
	if size1 != int64(s1Size) {
		t.Fatalf("expected size to be %v but was %v", s1Size, size1)
	}
	if size2 != int64(s2Size) {
		t.Fatalf("expected size to be %v but was %v", s2Size, size2)
	}
	if size3 != int64(s2Size) {
		t.Fatalf("expected size to be %v but was %v", s2Size, size3)
	}
	if size != size1+size2+size3 {
		t.Fatalf("total size should be %v but was %v", size, size1+size2+size3)
	}

	if !bytes.Equal(s1Data, readS1Data) {
		t.Fatal("the read data doesn't match the written data")
	}
	if !bytes.Equal(s2Data, readS2Data) {
		t.Fatal("the read data doesn't match the written data")
	}
	if !bytes.Equal(s3Data, readS3Data) {
		t.Fatal("the read data doesn't match the written data")
	}

	// Read the written data directly from the underlying file and check again.
	_, err = testFile.ReadAt(readS1Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testFile.ReadAt(readS2Data, int64(s1Size))
	if err != nil {
		t.Fatal(err)
	}
	_, err = testFile.ReadAt(readS3Data, int64(s1Size+s2Size))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(s1Data, readS1Data) {
		t.Fatal("the read data doesn't match the written data")
	}
	if !bytes.Equal(s2Data, readS2Data) {
		t.Fatal("the read data doesn't match the written data")
	}
	if !bytes.Equal(s3Data, readS3Data) {
		t.Fatal("the read data doesn't match the written data")
	}
}

// TestFileSectionBoundariesInvalidReadWrites tries a variation of invalid read
// and write operations on the section to make sure the caller can't write to
// neighboring sections by accident.
func TestFileSectionBoundariesInvalidReadWrites(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir(t.Name())
	if err := os.MkdirAll(testDir, 0700); err != nil {
		t.Fatal(err)
	}
	testFile, err := os.Create(filepath.Join(testDir, "testfile.dat"))
	if err != nil {
		t.Fatal(err)
	}

	// Create 3 sections.
	s1Size := 100
	s2Size := 100
	s1 := newFileSection(testFile, 0, int64(s1Size))
	s2 := newFileSection(testFile, int64(s1Size), int64(s1Size+s2Size))

	// Fill the file with some random data
	randomData := fastrand.Bytes(s1Size + s2Size + 100)
	if _, err := testFile.WriteAt(randomData, 0); err != nil {
		t.Fatal(err)
	}
	// Create some random data for the following calls to write. That data
	// should never be written since all the calls should fail.
	data := fastrand.Bytes(1)

	// Try a number of invalid read and write operations. They should all fail.
	if _, err := s1.SafeWriteAt(data, int64(s1Size)); err == nil {
		t.Fatal("sector shouldn't be able to write data beyond its end boundary")
	}
	if _, err := s2.SafeWriteAt(data, -1); err == nil {
		t.Fatal("sector shouldn't be able to write data below its start boundary")
	}
	if _, err := s1.SafeReadAt(data, int64(s1Size)); err == nil {
		t.Fatal("sector shouldn't be able to read data beyond its end boundary")
	}
	if _, err := s2.SafeReadAt(data, -1); err == nil {
		t.Fatal("sector shouldn't be able to read data below its start boundary")
	}

	// The file should still have the same random data from the beginning.
	fileData, err := ioutil.ReadAll(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileData, randomData) {
		t.Fatal("file data doesn't match the initial data")
	}
}

// TestFileSectionTruncate checks if file sections without an open end boundary
// can be truncated and makes sure that the last section can't truncate the
// file below its boundary.
func TestFileSectionTruncate(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir(t.Name())
	if err := os.MkdirAll(testDir, 0700); err != nil {
		t.Fatal(err)
	}
	testFile, err := os.Create(filepath.Join(testDir, "testfile.dat"))
	if err != nil {
		t.Fatal(err)
	}

	// Create 3 sections.
	s1Size := 100
	s2Size := 100
	s1 := newFileSection(testFile, 0, int64(s1Size))
	s2 := newFileSection(testFile, int64(s1Size), int64(s1Size+s2Size))
	s3 := newFileSection(testFile, int64(s1Size+s2Size), remainingFile)

	// Write as much data to the sections as they can fit.
	s1Data := fastrand.Bytes(s1Size)
	s2Data := fastrand.Bytes(s2Size)
	s3Data := fastrand.Bytes(s2Size) // s3 has an infinite size so we just write s2Size bytes.

	n, err := s1.SafeWriteAt(s1Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(s1Data) {
		t.Fatalf("expected %v bytes to be written instead of %v", len(s1Data), n)
	}
	n, err = s2.SafeWriteAt(s2Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(s2Data) {
		t.Fatalf("expected %v bytes to be written instead of %v", len(s2Data), n)
	}
	n, err = s3.SafeWriteAt(s3Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(s3Data) {
		t.Fatalf("expected %v bytes to be written instead of %v", len(s3Data), n)
	}

	// Try to truncate s1 and s2. That shouldn't be possible.
	if err := s1.SafeTruncate(int64(fastrand.Intn(s1Size + 1))); err == nil {
		t.Fatal("it shouldn't be possible to truncate a section with a fixed end boundary.")
	}
	if err := s2.SafeTruncate(int64(fastrand.Intn(s2Size + 1))); err == nil {
		t.Fatal("it shouldn't be possible to truncate a section with a fixed end boundary.")
	}

	// Try to truncate s3 to size 0. This should be possible and also reduce
	// the total file size.
	if err := s3.SafeTruncate(0); err != nil {
		t.Fatal("failed to truncate s3", err)
	}
	fi, err := testFile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != int64(s1Size+s2Size) {
		t.Fatalf("expected size after truncate is %v but was %v", s1Size+s2Size, fi.Size())
	}
	size, err := s3.Size()
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("size was %v but should be %v", size, 0)
	}
	// Try to truncate s3 below its start. That shouldn't be possible.
	if err := s3.SafeTruncate(-1); err == nil {
		t.Fatal("it shouldn't be possible to truncate a section to a negative size")
	}
}
