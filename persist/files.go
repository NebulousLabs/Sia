package persist

import (
	"encoding/json"
	"errors"
	"os"
)

var (
	ErrBadVersion = errors.New("incompatible version")
	ErrBadHeader  = errors.New("wrong header")
)

type Metadata struct {
	Header, Version, Filename string
}

// Save saves data to disk.
func Save(meta Metadata, data interface{}) error {
	file, err := os.Create(meta.Filename)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	enc := json.NewEncoder(file)
	if err := enc.Encode(meta.Header); err != nil {
		return err
	}
	if err := enc.Encode(meta.Version); err != nil {
		return err
	}
	if _, err = file.Write(b); err != nil {
		return err
	}

	return nil
}

// Load loads data from disk.
func Load(meta Metadata, data interface{}) error {
	file, err := os.Open(meta.Filename)
	if err != nil {
		return err
	}

	var header, version string
	dec := json.NewDecoder(file)
	if err := dec.Decode(&header); err != nil {
		return err
	}
	if header != meta.Header {
		return ErrBadHeader
	}
	if err := dec.Decode(&version); err != nil {
		return err
	}
	if version != meta.Version {
		return ErrBadVersion
	}
	if err := dec.Decode(data); err != nil {
		return err
	}

	return nil
}
