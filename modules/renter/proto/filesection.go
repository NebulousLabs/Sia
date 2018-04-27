package proto

import (
	"os"
)

// fileSection is a helper struct that is used to split a file up in multiple
// sections. This guarantees that each part of the file can only write to and
// read from its corresponding section.
type fileSection struct {
	f     *os.File
	start int64
	end   int64
}

// newFileSection creates a new fileSection from a file and the provided bounds
// of the section.
func newFileSection(f *os.File, start, end int64) *fileSection {
	if start < 0 {
		panic("filesection can't start at an index < 0")
	}
	if end < start && end != remainingFile {
		panic("the end of a filesection can't be before the start")
	}
	return &fileSection{
		f:     f,
		start: start,
		end:   end,
	}
}

// Close calls Close on the fileSection's underlying file.
func (f *fileSection) Close() error {
	return f.f.Close()
}

// Size uses the underlying file's stats to return the size of the sector.
func (f *fileSection) Size() (int64, error) {
	info, err := f.f.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size() - f.start
	if size < 0 {
		size = 0
	}
	if size > f.end-f.start && f.end != remainingFile {
		size = f.end - f.start
	}
	return size, nil
}

// ReadAt calls the fileSection's underlying file's ReadAt method if the read
// happens within the bounds of the section. Otherwise it returns io.EOF.
func (f *fileSection) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 {
		panic("can't read from an offset before the section start")
	}
	if f.start+off+int64(len(b)) > f.end && f.end != remainingFile {
		panic("can't read from an offset after the section end")
	}
	return f.f.ReadAt(b, f.start+off)
}

// Sync calls Sync on the fileSection's underlying file.
func (f *fileSection) Sync() error {
	return f.f.Sync()
}

// Truncate calls Truncate on the fileSection's underlying file.
func (f *fileSection) Truncate(size int64) error {
	if f.end != remainingFile {
		panic("can't truncate a file that has a end != remainingFile")
	}
	if f.start+size < f.start {
		panic("can't truncate file to be smaller than the section start")
	}
	return f.f.Truncate(f.start + size)
}

// WriteAt calls the fileSection's underlying file's WriteAt method if the
// write happens within the bounds of the section. Otherwise it returns io.EOF.
func (f *fileSection) WriteAt(b []byte, off int64) (int, error) {
	if off < 0 {
		panic("can't read from an offset before the section start")
	}
	if f.start+off+int64(len(b)) > f.end && f.end != remainingFile {
		panic("can't read from an offset after the section end")
	}
	return f.f.WriteAt(b, off+f.start)
}
