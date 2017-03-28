package persist

import (
	"encoding/json"
	"io"
	"os"
)

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

// LoadFile loads json data from a file.
func LoadFile(meta Metadata, data interface{}, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	err = Load(meta, data, file)
	if err != nil {
		return err
	}
	return nil
}

// Save saves json data to a writer.
func Save(meta Metadata, data interface{}, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	if err := enc.Encode(meta.Header); err != nil {
		return err
	}
	if err := enc.Encode(meta.Version); err != nil {
		return err
	}
	if err := enc.Encode(data); err != nil {
		return err
	}

	return nil
}

// SaveFile atomically saves json data to a file.
func SaveFile(meta Metadata, data interface{}, filename string) error {
	file, err := NewSafeFile(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	err = Save(meta, data, file)
	if err != nil {
		return err
	}
	return file.Commit()
}

// SaveFileSync atomically saves json data to a file and then syncs to disk.
func SaveFileSync(meta Metadata, data interface{}, filename string) error {
	file, err := NewSafeFile(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	err = Save(meta, data, file)
	if err != nil {
		return err
	}
	return file.CommitSync()
}
