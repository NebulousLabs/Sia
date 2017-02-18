package contractor

// The contractor achieves efficient persistence using a JSON transaction
// journal. It enables efficient ACID transactions on JSON objects.
//
// Each journal represents a single JSON object. The object is serialized as
// an "initial object" followed by a series of update sets, one per line. Each
// update specifies a field and a modification. See the journalUpdate type for
// a full specification.
//
// During operation, the object is first loaded by reading the file and
// applying each update to the initial object. It is subsequently modified by
// appending update sets to the file, one per line. At any time, a
// "checkpoint" may be created, which clears the journal and starts over with
// a new initial object. This allows for compaction of the journal file.
//
// In the event of power failure or other serious disruption, the most recent
// update set may be only partially written. Partially written update sets are
// simply ignored when reading the journal. Individual updates may also be
// ignored if they are malformed, though other updates in the set may be
// applied. See the journalUpdate docstring for an explanation of malformed updates.

// TODO:
// - handle case sensitivity
// - handle null vs. empty array
//    - if index is 0, just do a wholesale replace with [val]
// - handle inserting new keys into object

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
)

// A journal is a log of updates to a JSON object.
type journal struct {
	f        *os.File
	filename string
}

// update applies the updates atomically to j. It syncs the underlying file
// before returning.
func (j *journal) update(us []journalUpdate) error {
	buf := make([]byte, 0, 1024) // reasonable guess; avoids GC if we're lucky
	buf = append(buf, '[')
	for i, u := range us {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, `{"p":"`...)
		buf = append(buf, u.Path...)
		buf = append(buf, `","v":`...)
		buf = append(buf, *u.Value...)
		buf = append(buf, '}')
	}
	buf = append(buf, ']', '\n')
	if _, err := j.f.Write(buf); err != nil {
		return err
	}
	return j.f.Sync()
}

// Checkpoint refreshes the journal with a new initial object. It syncs the
// underlying file before returning.
func (j *journal) checkpoint(obj interface{}) error {
	// write to a new temp file
	//
	// TODO: a separate file may not be necessary. We could use an update with
	// path "" instead, and then overwrite the beginning of the file and
	// truncate. If the overwrite fails, we still have the full rewrite update
	// left at the end. Just need to be careful not to overflow into the
	// update if the new object is large.
	tmp, err := os.Create(j.filename + "_tmp")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(tmp).Encode(obj); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}

	// atomically replace the old file with the new one
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := j.f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), j.filename); err != nil {
		return err
	}

	// reopen the journal
	j.f, err = os.OpenFile(j.filename, os.O_RDWR|os.O_APPEND, 0)
	return err
}

// Close closes the underlying file.
func (j *journal) Close() error {
	return j.f.Close()
}

// openJournal opens the supplied journal and decodes the reconstructed object
// into obj. If the journal does not exist, it will be created and obj will be
// used as the initial object.
func openJournal(filename string, obj interface{}) (*journal, error) {
	// open file handle, creating the file if it does not exist
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	// if file was newly created, use obj as the initial object.
	if stat, err := f.Stat(); err != nil {
		return nil, err
	} else if stat.Size() == 0 {
		j := &journal{
			f:        f,
			filename: filename,
		}
		if err := json.NewEncoder(f).Encode(obj); err != nil {
			return nil, err
		}
		if err := f.Sync(); err != nil {
			return nil, err
		}
		return j, nil
	}

	// decode the initial object
	var initObj json.RawMessage
	dec := json.NewDecoder(f)
	if err = dec.Decode(&initObj); err != nil {
		return nil, err
	}
	// decode each set of updates
	for {
		var set []journalUpdate
		if err = dec.Decode(&set); err == io.EOF || err == io.ErrUnexpectedEOF {
			// unexpected EOF means the last update was corrupted
			break
		} else if _, ok := err.(*json.SyntaxError); ok {
			// skip malformed update sets
			continue
		} else if err != nil {
			return nil, err
		}
		for _, u := range set {
			initObj = u.apply(initObj)
		}
	}
	// decode the final object into obj
	if err = json.Unmarshal(initObj, obj); err != nil {
		return nil, err
	}

	return &journal{
		f:        f,
		filename: filename,
	}, nil
}

// A journalUpdate is a modification of a path in a JSON object. A "path" in this
// context means an object or array element. Syntactically, a path is a set of
// accessors joined by the '.' character. An accessor is either an object key
// or an array index. For example, given this object:
//
//    {
//        "foo": {
//            "bars": [
//                {"baz":3}
//            ]
//        }
//    }
//
// The following path accesses the value "3":
//
//    foo.bars.0.baz
//
// The path is accompanied by a new object. Thus, to increment the value "3"
// in the above object, we would use the following journalUpdate:
//
//    {
//        "p": "foo.bars.0.baz",
//        "v": 4
//    }
//
// All permutations of the journalUpdate object are legal. However, malformed updates
// are ignored during application. A journalUpdate is considered malformed in three
// circumstances:
//
// - Its Path references an element that does not exist at application time.
//   This includes out-of-bounds array indices.
// - Its Path contains invalid characters (e.g. "). See the JSON spec.
// - Value contains invalid JSON or is empty.
//
// Other special cases are handled as follows:
//
// - If Path is "", the entire object is replaced.
// - If an object contains duplicate keys, the first key encountered is used.
//
// Finally, to enable efficient array updates, the length of the array (at
// application time) may be used as a special array index.  When this index is
// the last accessor in Path, Value will be appended to the end of the array.
// If the index is not the last accessor, the journalUpdate is considered malformed
// (and thus is ignored).
type journalUpdate struct {
	// Path is an arbitrarily-nested JSON element, such as foo.bars.1.baz
	Path string `json:"p"`
	// Value contains the new value of Path.
	// TODO: remove pointer once Go 1.8 is released.
	Value *json.RawMessage `json:"v"`
}

// apply applies u to obj, returning the new JSON, which may share underlying
// memory with obj or u.Value. If u is malformed, obj is returned unaltered.
// See the journalUpdate docstring for an explanation of malformed journalUpdates. If obj is
// not valid JSON, the result is undefined.
func (u journalUpdate) apply(obj json.RawMessage) json.RawMessage {
	if len(*u.Value) == 0 {
		// u is malformed
		return obj
	}
	return rewritePath(obj, u.Path, *u.Value)
}

// newJournalUpdate constructs an update using the provided path and val. If val
// cannot be marshaled, newJournalUpdate panics. If val implements the json.Marshaler
// interface, it is called directly. Note that this bypasses validation of the
// produced JSON, which may result in a malformed journalUpdate.
func newJournalUpdate(path string, val interface{}) journalUpdate {
	var data []byte
	var err error
	if m, ok := val.(json.Marshaler); ok {
		// bypass validation
		data, err = m.MarshalJSON()
	} else {
		data, err = json.Marshal(val)
	}
	if err != nil {
		panic(err)
	}
	rm := json.RawMessage(data)
	return journalUpdate{
		Path:  path,
		Value: &rm,
	}
}

// rewritePath replaces the value at path in json with val. The returned slice
// may share underlying memory with json. If path is malformed, the original
// json is returned.
func rewritePath(json []byte, path string, val []byte) []byte {
	if path == "" {
		return val
	}

	var lastAcc string
	var i int
	for j := 0; lastAcc == ""; j++ {
		// determine next accessor by seeking to .
		dotIndex := strings.IndexByte(path[j:], '.')
		if dotIndex == -1 {
			// not found; this is the last accessor
			dotIndex = len(path[j:])
			lastAcc = path[j:]
		}
		acc := path[j : j+dotIndex]
		j += dotIndex

		// seek to accessor
		accIndex := locateAccessor(json[i:], acc)
		if accIndex == -1 {
			// not found; return unmodified
			return json
		} else if json[accIndex] == ']' && lastAcc == "" {
			// only the last accessor may use the "append" index
			return json
		}
		i += accIndex
	}

	// replace old value
	newJSON := make([]byte, 0, len(json)+len(val)) // reasonable guess
	newJSON = append(newJSON, json[:i]...)
	if json[i] == ']' {
		// we are appending. If the array is not empty, insert an extra ,
		if lastAcc != "0" {
			newJSON = append(newJSON, ',')
		}
	}
	newJSON = append(newJSON, val...)
	newJSON = append(newJSON, consumeValue(json[i:])...)

	return newJSON
}

// locateAccessor returns the offset of acc in json.
func locateAccessor(json []byte, acc string) int {
	origLen := len(json)
	json = consumeWhitespace(json)
	if len(json) == 0 || len(json) < len(acc) {
		return -1
	}

	// acc must refer to either an object key or an array index. So if we
	// don't see a { or [, the path is invalid.
	switch json[0] {
	default:
		return -1

	case '{': // object
		json = consumeSeparator(json) // consume {
		// iterate through keys, searching for acc
		for json[0] != '}' {
			var key []byte
			key, json = parseString(json)
			json = consumeWhitespace(json)
			json = consumeSeparator(json) // consume :
			if bytes.Equal(key, []byte(acc)) {
				// acc found
				return origLen - len(json)
			}
			json = consumeValue(json)
			json = consumeWhitespace(json)
			if json[0] == ',' {
				json = consumeSeparator(json) // consume ,
			}
		}
		// acc not found
		return -1

	case '[': // array
		// is accessor possibly an array index?
		n, err := strconv.Atoi(acc)
		if err != nil || n < 0 {
			// invalid index
			return -1
		}
		json = consumeSeparator(json) // consume [
		// consume n keys, stopping early if we hit the end of the array
		var arrayLen int
		for n > arrayLen && json[0] != ']' {
			json = consumeValue(json)
			arrayLen++
			json = consumeWhitespace(json)
			if json[0] == ',' {
				json = consumeSeparator(json) // consume ,
			}
		}
		if n > arrayLen {
			// Note that n == arrayLen is allowed. In this case, an append
			// operation is desired; we return the offset of the closing ].
			return -1
		}
		return origLen - len(json)
	}
}

func parseString(json []byte) ([]byte, []byte) {
	after := consumeString(json)
	strLen := len(json) - len(after) - 2
	return json[1 : 1+strLen], after
}

func consumeWhitespace(json []byte) []byte {
	for i := range json {
		if c := json[i]; c > ' ' || (c != ' ' && c != '\t' && c != '\n' && c != '\r') {
			return json[i:]
		}
	}
	return json[len(json):]
}

func consumeSeparator(json []byte) []byte {
	json = json[1:] // consume one of [ { } ] : ,
	return consumeWhitespace(json)
}

func consumeValue(json []byte) []byte {
	// determine value type
	switch json[0] {
	case '{': // object
		return consumeObject(json)
	case '[': // array
		return consumeArray(json)
	case '"': // string
		return consumeString(json)
	case 't', 'n': // true or null
		return json[4:]
	case 'f': // false
		return json[5:]
	default: // number
		return consumeNumber(json)
	}
}

func consumeObject(json []byte) []byte {
	json = json[1:] // consume {
	// seek to next {, }, or ". Each time we encounter a {, increment n. Each
	// time encounter a }, decrement n. Exit when n == 0. If we encounter ",
	// consume the string.
	n := 1
	for n > 0 {
		json = json[bytes.IndexAny(json, `{}"`):]
		switch json[0] {
		case '{':
			n++
			json = json[1:] // consume {
		case '}':
			n--
			json = json[1:] // consume }
		case '"':
			json = consumeString(json)
		}
	}
	return json
}

func consumeArray(json []byte) []byte {
	json = json[1:] // consume [
	// seek to next [, ], or ". Each time we encounter a [, increment n. Each
	// time encounter a ], decrement n. Exit when n == 0. If we encounter ",
	// consume the string.
	n := 1
	for n > 0 {
		json = json[bytes.IndexAny(json, `[]"`):]
		switch json[0] {
		case '[':
			n++
			json = json[1:] // consume [
		case ']':
			n--
			json = json[1:] // consume ]
		case '"':
			json = consumeString(json)
		}
	}
	return json
}

func consumeString(json []byte) []byte {
	i := 1 // consume "
	// seek forward until we find a " without a preceeding \
	i += bytes.IndexByte(json[i:], '"')
	for json[i-1] == '\\' {
		i++
		i += bytes.IndexByte(json[i:], '"')
	}
	return json[i+1:] // consume "
}

func consumeNumber(json []byte) []byte {
	if json[0] == '-' {
		json = json[1:]
	}
	// leading digits
	for '0' <= json[0] && json[0] <= '9' {
		json = json[1:]
		if len(json) == 0 {
			return json
		}
	}
	// decimal digits
	if json[0] == '.' {
		json = json[1:]
		for '0' <= json[0] && json[0] <= '9' {
			json = json[1:]
			if len(json) == 0 {
				return json
			}
		}
	}
	// exponent
	if json[0] == 'e' || json[0] == 'E' {
		json = json[1:]
		if json[0] == '+' || json[0] == '-' {
			json = json[1:]
		}
		for '0' <= json[0] && json[0] <= '9' {
			json = json[1:]
			if len(json) == 0 {
				return json
			}
		}
	}
	return json
}
