package persist

// NOTE: The safe json files include a checksum that is allowed to be manually
// overwritten by the user. This temporarily exposes the user to corruption, not
// just from a json file that has the wrong values, but if the disk fails right
// after the user has manually modified their json file, there are edge cases
// where because of the manual checksum, the saver will not be able to detect
// corruption.

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/crypto"

	"gitlab.com/NebulousLabs/errors"
)

// verifyChecksum will disregard the metadata of the saved file, and just verify
// that the checksum matches the data below the checksum to be certain that the
// file is correct.
func verifyChecksum(filename string) bool {
	// Open the file.
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		// No file at all means that everything is okay. This is a condition we
		// are going to hit the first time that we ever save a file.
		return true
	}
	if err != nil {
		// An error opening the file means that the checksum verification has
		// failed, we don't have confidence that this a a good file.
		return false
	}
	defer file.Close()

	// Read the metadata from the file. This is not covered by the checksum but
	// we have to read it anyway to get to the checksum.
	var header, version string
	dec := json.NewDecoder(file)
	if err := dec.Decode(&header); err != nil {
		return false
	}
	if err := dec.Decode(&version); err != nil {
		return false
	}

	// Read everything else.
	remainingBytes, err := ioutil.ReadAll(dec.Buffered())
	if err != nil {
		return false
	}
	// The buffer may or may not have read the rest of the file, read the rest
	// of the file to be certain.
	remainingBytesExtra, err := ioutil.ReadAll(file)
	if err != nil {
		return false
	}
	remainingBytes = append(remainingBytes, remainingBytesExtra...)

	// Determine whether the leading bytes contain a checksum. A proper checksum
	// will be 67 bytes (quote, 64 byte checksum, quote, newline). A manual
	// checksum will be the characters "manual\n" (9 characters). If neither
	// decode correctly, it is assumed that there is no checksum at all.
	var checksum crypto.Hash
	if len(remainingBytes) >= 67 {
		err = json.Unmarshal(remainingBytes[:67], &checksum)
		if err == nil {
			// The checksum was read successfully. Return 'true' if the checksum
			// matches the remaining data, and false otherwise.
			return checksum == crypto.HashBytes(remainingBytes[68:])
		}
	}

	// The checksum was not read correctly, check if the next few bytes are
	// the "manual" checksum.
	var manualChecksum string
	if len(remainingBytes) >= 9 {
		err = json.Unmarshal(remainingBytes[:9], &manualChecksum)
		if err == nil && manualChecksum == "manual" {
			return true
		}
	}

	// The checksum could not be decoded. Older versions of the file did not
	// have a checksum, but the remaining data would still need to be valid
	// JSON. If we are this far, it means that either the file is corrupt, or it
	// is an old file where all remaining bytes should be valid json.
	return json.Valid(remainingBytes)
}

// readJSON will try to read a persisted json object from a file.
func readJSON(meta Metadata, object interface{}, filename string) error {
	// Open the file.
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		return err
	}
	if err != nil {
		return build.ExtendErr("unable to open persisted json object file", err)
	}
	defer file.Close()

	// Read the metadata from the file.
	var header, version string
	dec := json.NewDecoder(file)
	if err := dec.Decode(&header); err != nil {
		return build.ExtendErr("unable to read header from persisted json object file", err)
	}
	if header != meta.Header {
		return ErrBadHeader
	}
	if err := dec.Decode(&version); err != nil {
		return build.ExtendErr("unable to read version from persisted json object file", err)
	}
	if version != meta.Version {
		return ErrBadVersion
	}

	// Read everything else.
	remainingBytes, err := ioutil.ReadAll(dec.Buffered())
	if err != nil {
		return build.ExtendErr("unable to read persisted json object data", err)
	}
	// The buffer may or may not have read the rest of the file, read the rest
	// of the file to be certain.
	remainingBytesExtra, err := ioutil.ReadAll(file)
	if err != nil {
		return build.ExtendErr("unable to read persisted json object data", err)
	}
	remainingBytes = append(remainingBytes, remainingBytesExtra...)

	// Determine whether the leading bytes contain a checksum. A proper checksum
	// will be 67 bytes (quote, 64 byte checksum, quote, newline). A manual
	// checksum will be the characters "manual\n" (9 characters). If neither
	// decode correctly, it is assumed that there is no checksum at all.
	checkManual := len(remainingBytes) >= 9
	if len(remainingBytes) >= 67 {
		var checksum crypto.Hash
		err = json.Unmarshal(remainingBytes[:67], &checksum)
		checkManual = checkManual && err != nil
		if err == nil && checksum != crypto.HashBytes(remainingBytes[68:]) {
			return errors.New("loading a file with a bad checksum")
		} else if err == nil {
			remainingBytes = remainingBytes[68:]
		}
	}

	// checkManual will be set to true so long as the remainingBytes is at least
	// 9 bytes long, and also there was an error when parsing the checksum. The
	// manual checksum is considered correct if the json unmarshalling parses
	// correctly, and also the bytes match the string "manual".
	if checkManual {
		var manualChecksum string
		err := json.Unmarshal(remainingBytes[:9], &manualChecksum)
		if err == nil && manualChecksum != "manual" {
			return errors.New("loading a file with a bad checksum")
		} else if err == nil {
			remainingBytes = remainingBytes[10:]
		}
	}

	// Any valid checksum has been stripped off. If there was an invalid
	// checksum, an error has been returned. There is also the case that no
	// checksum was written at all, which is ignored as a case - it's needed to
	// preserve compatibility with previous persist files.

	// Parse the json object.
	return json.Unmarshal(remainingBytes, &object)
}

// LoadJSON will load a persisted json object from disk.
func LoadJSON(meta Metadata, object interface{}, filename string) error {
	// Verify that the filename does not have the persist temp suffix.
	if strings.HasSuffix(filename, tempSuffix) {
		return ErrBadFilenameSuffix
	}

	// Verify that no other thread is using this filename.
	err := func() error {
		activeFilesMu.Lock()
		defer activeFilesMu.Unlock()

		_, exists := activeFiles[filename]
		if exists {
			build.Critical(ErrFileInUse, filename)
			return ErrFileInUse
		}
		activeFiles[filename] = struct{}{}
		return nil
	}()
	if err != nil {
		return err
	}
	// Release the lock at the end of the function.
	defer func() {
		activeFilesMu.Lock()
		delete(activeFiles, filename)
		activeFilesMu.Unlock()
	}()

	// Try opening the primary file.
	err = readJSON(meta, object, filename)
	if err == ErrBadHeader || err == ErrBadVersion || os.IsNotExist(err) {
		return err
	}
	if err != nil {
		// Try opening the temp file.
		err := readJSON(meta, object, filename+tempSuffix)
		if err != nil {
			return build.ExtendErr("unable to read persisted json object from disk", err)
		}
	}

	// Success.
	return nil
}

// SaveJSON will save a json object to disk in a durable, atomic way. The
// resulting file will have a checksum of the data as the third line. If
// manually editing files, the checksum line can be replaced with the 8
// characters "manual". This will cause the reader to accept the checksum even
// though the file has been changed.
func SaveJSON(meta Metadata, object interface{}, filename string) error {
	// Verify that the filename does not have the persist temp suffix.
	if strings.HasSuffix(filename, tempSuffix) {
		return ErrBadFilenameSuffix
	}

	// Verify that no other thread is using this filename.
	err := func() error {
		activeFilesMu.Lock()
		defer activeFilesMu.Unlock()

		_, exists := activeFiles[filename]
		if exists {
			build.Critical(ErrFileInUse, filename)
			return ErrFileInUse
		}
		activeFiles[filename] = struct{}{}
		return nil
	}()
	if err != nil {
		return err
	}
	// Release the lock at the end of the function.
	defer func() {
		activeFilesMu.Lock()
		delete(activeFiles, filename)
		activeFilesMu.Unlock()
	}()

	// Write the metadata to the buffer.
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(meta.Header); err != nil {
		return build.ExtendErr("unable to encode metadata header", err)
	}
	if err := enc.Encode(meta.Version); err != nil {
		return build.ExtendErr("unable to encode metadata version", err)
	}

	// Marshal the object into json and write the checksum + result to the
	// buffer.
	objBytes, err := json.MarshalIndent(object, "", "\t")
	if err != nil {
		return build.ExtendErr("unable to marshal the provided object", err)
	}
	checksum := crypto.HashBytes(objBytes)
	if err := enc.Encode(checksum); err != nil {
		return build.ExtendErr("unable to encode checksum", err)
	}
	buf.Write(objBytes)
	data := buf.Bytes()

	// Write out the data to the temp file, with a sync.
	err = func() (err error) {
		// Verify the checksum of the real file. If the real file does not have
		// a valid checksum, we do not want to risk overwriting the temp file,
		// which may be the only good version of the persistence remaining.
		// We'll skip writing the temp file to make sure it stays intact, and go
		// straight to over-writing the real file.
		if !verifyChecksum(filename) {
			return nil
		}

		file, err := os.OpenFile(filename+tempSuffix, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0600)
		if err != nil {
			return build.ExtendErr("unable to open temp file", err)
		}
		defer func() {
			err = build.ComposeErrors(err, file.Close())
		}()

		// Write and sync.
		_, err = file.Write(data)
		if err != nil {
			return build.ExtendErr("unable to write temp file", err)
		}
		err = file.Sync()
		if err != nil {
			return build.ExtendErr("unable to sync temp file", err)
		}
		return nil
	}()
	if err != nil {
		return err
	}

	// Write out the data to the real file, with a sync.
	err = func() (err error) {
		file, err := os.OpenFile(filename, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0600)
		if err != nil {
			return build.ExtendErr("unable to open file", err)
		}
		defer func() {
			err = build.ComposeErrors(err, file.Close())
		}()

		// Write and sync.
		_, err = file.Write(data)
		if err != nil {
			return build.ExtendErr("unable to write file", err)
		}
		err = file.Sync()
		if err != nil {
			return build.ExtendErr("unable to sync temp file", err)
		}
		return nil
	}()
	if err != nil {
		return err
	}

	// Success
	return nil
}
