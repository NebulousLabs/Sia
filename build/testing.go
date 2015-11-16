package build

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

var (
	// SiaTestingDir is the directory that contains all of the files and
	// folders created during testing.
	SiaTestingDir = filepath.Join(os.TempDir(), "SiaTesting")
)

// TempDir joins the provided directories and prefixes them with the Sia
// testing directory.
func TempDir(dirs ...string) string {
	path := filepath.Join(SiaTestingDir, filepath.Join(dirs...))
	os.RemoveAll(path) // remove old test data
	return path
}

// CopyFile copies a file from a source to a destination.
func Copyfile(source, dest string) error {
	sf, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, sf)
	if err != nil {
		return err
	}
	return nil
}

// CopyDir copies a directory and all of its contents to the destination
// directory.
func CopyDir(source, dest string) error {
	stat, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return errors.New("source is not a directory")
	}

	err = os.MkdirAll(dest, stat.Mode())
	if err != nil {
		return err
	}
	files, err := ioutil.ReadDir(source)
	for _, file := range files {
		newSource := filepath.Join(source, file.Name())
		newDest := filepath.Join(dest, file.Name())
		if file.IsDir() {
			err = CopyDir(newSource, newDest)
			if err != nil {
				return err
			}
		} else {
			err = Copyfile(newSource, newDest)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
