package persist

import (
	"encoding/json"
	"io"
	"os"
)

// Save saves json data to a writer.
func Save(meta Metadata, data interface{}, w io.Writer) error {
	b, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	if err := enc.Encode(meta.Header); err != nil {
		return err
	}
	if err := enc.Encode(meta.Version); err != nil {
		return err
	}
	if _, err = w.Write(b); err != nil {
		return err
	}

	return nil
}

// Load loads json data from a reader.
func Load(meta Metadata, data interface{}, r io.Reader) error {
	var header, version string
	dec := json.NewDecoder(r)
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

// SaveFile saves json data to a file.
func SaveFile(meta Metadata, data interface{}, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return Save(meta, data, file)
}

// LoadFile loads json data from a file.
func LoadFile(meta Metadata, data interface{}, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return Load(meta, data, file)
}
