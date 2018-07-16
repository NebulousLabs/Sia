package persist

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/fastrand"
	"github.com/coreos/bbolt"
)

// testInputs and testFilenames are global variables because most tests require
// a variety of metadata and filename inputs (although only TestCheckMetadata
// and TestIntegratedCheckMetadata use testInput.newMd and testInput.err).
// Weird strings are from https://github.com/minimaxir/big-ltist-of-naughty-strings
var (
	testInputs = []struct {
		md    Metadata
		newMd Metadata
		err   error
	}{
		{Metadata{"1sadf23", "12253"}, Metadata{"1sa-df23", "12253"}, ErrBadHeader},
		{Metadata{"$@#$%^&", "$@#$%^&"}, Metadata{"$@#$%^&", "$@#$%!^&"}, ErrBadVersion},
		{Metadata{"//", "//"}, Metadata{"////", "//"}, ErrBadHeader},
		{Metadata{":]", ":)"}, Metadata{":]", ":("}, ErrBadVersion},
		{Metadata{"¯|_(ツ)_|¯", "_|¯(ツ)¯|_"}, Metadata{"¯|_(ツ)_|¯", "_|¯(ツ)_|¯"}, ErrBadVersion},
		{Metadata{"世界", "怎么办呢"}, Metadata{"世界", "怎么好呢"}, ErrBadVersion},
		{Metadata{"     ", "     "}, Metadata{"\t", "     "}, ErrBadHeader},
		{Metadata{"", ""}, Metadata{"asdf", ""}, ErrBadHeader},
		{Metadata{"", "_"}, Metadata{"", ""}, ErrBadVersion},
		{Metadata{"%&*", "#@$"}, Metadata{"", "#@$"}, ErrBadHeader},
		{Metadata{"a.sdf", "0.30.2"}, Metadata{"a.sdf", "0.3.02"}, ErrBadVersion},
		{Metadata{"/", "/"}, Metadata{"//", "/"}, ErrBadHeader},
		{Metadata{"%*.*s", "%d"}, Metadata{"%*.*s", "%    d"}, ErrBadVersion},
		{Metadata{" ", ""}, Metadata{"   ", ""}, ErrBadHeader},
		{Metadata{"⒯⒣⒠ ⒬⒰⒤⒞⒦ ⒝⒭⒪⒲⒩ ⒡⒪⒳ ⒥⒰⒨⒫⒮ ⒪⒱⒠⒭ ⒯⒣⒠ ⒧⒜⒵⒴ ⒟⒪⒢", "undefined"}, Metadata{"⒯⒣⒠ ⒬⒰⒤⒞⒦ ⒝⒭⒪⒲⒩ ⒡⒪⒳ ⒥⒰⒨⒫⒮ ⒪⒱⒠⒭ ⒯⒣⒠ ⒧⒜⒵⒴ ⒟⒪⒢", "␢undefined"}, ErrBadVersion},
		{Metadata{" ", "  "}, Metadata{"  ", "  "}, ErrBadHeader},
		{Metadata{"\xF0\x9F\x98\x8F", "\xF0\x9F\x98\xBE"}, Metadata{"\xF0\x9F\x98\x8F", " \xF0\x9F\x98\xBE"}, ErrBadVersion},
		{Metadata{"'", ""}, Metadata{"`", ""}, ErrBadHeader},
		{Metadata{"", "-"}, Metadata{"", "-␡"}, ErrBadVersion},
		{Metadata{"<foo val=“bar” />", "(ﾉಥ益ಥ ┻━┻"}, Metadata{"<foo val=“bar” />", "(ﾉ\nಥ益ಥ ┻━┻"}, ErrBadVersion},
		{Metadata{"\n\n", "Ṱ̺̺o͞ ̷i̲̬n̝̗v̟̜o̶̙kè͚̮ ̖t̝͕h̼͓e͇̣ ̢̼h͚͎i̦̲v̻͍e̺̭-m̢iͅn̖̺d̵̼ ̞̥r̛̗e͙p͠r̼̞e̺̠s̘͇e͉̥ǹ̬͎t͍̬i̪̱n͠g̴͉ ͏͉c̬̟h͡a̫̻o̫̟s̗̦.̨̹"}, Metadata{"\n\n", "Ṱ̺̺o͞ ̷i̲̬n̝̗v̟̜o̶̙kè͚̮ t̝͕h̼͓e͇̣ ̢̼h͚͎i̦̲v̻͍e̺̭-m̢iͅn̖̺d̵̼ ̞̥r̛̗e͙p͠r̼̞e̺̠s̘͇e͉̥ǹ̬͎t͍̬i̪̱n͠g̴͉ ͏͉c̬̟h͡a̫̻o̫̟s̗̦.̨̹"}, ErrBadVersion},
	}
	testFilenames = []string{
		"_",
		"-",
		"1234sg",
		"@#$%@#",
		"你好好q wgc好",
		"\xF0\x9F\x99\x8A",
		"␣",
		" ",
		"$HOME",
		",.;'[]-=",
		"%s",
	}
)

// TestOpenDatabase tests calling OpenDatabase on the following types of
// database:
// - a database that has not yet been created
// - an existing empty database
// - an existing nonempty database
// Along the way, it also tests calling Close on:
// - a newly-created database
// - a newly-filled database
// - a newly-emptied database
func TestOpenDatabase(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testBuckets := [][]byte{
		[]byte("Fake Bucket123!@#$"),
		[]byte("你好好好"),
		[]byte("¯|_(ツ)_|¯"),
		[]byte("Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗"),
		[]byte("﷽"),
		[]byte("(ﾉಥ益ಥ ┻━┻"),
		[]byte("Ṱ̺̺o͞ ̷i̲̬n̝̗v̟̜o̶̙kè͚̮ ̖t̝͕h̼͓e͇̣ ̢̼h͚͎i̦̲v̻͍e̺̭-m̢iͅn̖̺d̵̼ ̞̥r̛̗e͙p͠r̼̞e̺̠s̘͇e͉̥ǹ̬͎t͍̬i̪̱n͠g̴͉ ͏͉c̬̟h͡a̫̻o̫̟s̗̦.̨̹"),
		[]byte("0xbadidea"),
		[]byte("␣"),
		[]byte("你好好好"),
	}
	// Create a folder for the database file. If a folder by that name exists
	// already, it will be replaced by an empty folder.
	testDir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	for i, in := range testInputs {
		dbFilename := testFilenames[i%len(testFilenames)]
		dbFilepath := filepath.Join(testDir, dbFilename)
		// Create a new database.
		db, err := OpenDatabase(in.md, dbFilepath)
		if err != nil {
			t.Errorf("calling OpenDatabase on a new database failed for metadata %v, filename %v; error was %v", in.md, dbFilename, err)
			continue
		}
		// Close the newly-created, empty database.
		err = db.Close()
		if err != nil {
			t.Errorf("closing a newly created database failed for metadata %v, filename %v; error was %v", in.md, dbFilename, err)
			continue
		}
		// Call OpenDatabase again, this time on the existing empty database.
		db, err = OpenDatabase(in.md, dbFilepath)
		if err != nil {
			t.Errorf("calling OpenDatabase on an existing empty database failed for metadata %v, filename %v; error was %v", in.md, dbFilename, err)
			continue
		}
		// Create buckets in the database.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				_, err := tx.CreateBucketIfNotExists(testBucket)
				if err != nil {
					t.Errorf("db.Update failed on bucket name %v for metadata %v, filename %v; error was %v", testBucket, in.md, dbFilename, err)
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Error(err)
			continue
		}
		// Make sure CreateBucketIfNotExists method handles invalid (nil)
		// bucket name.
		err = db.Update(func(tx *bolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists(nil)
			return err
		})
		if err != bolt.ErrBucketNameRequired {
		}
		// Fill each bucket with a random number (0-9, inclusive) of key/value
		// pairs, where each key is a length-10 random byteslice and each value
		// is a length-1000 random byteslice.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				b := tx.Bucket(testBucket)
				x := fastrand.Intn(10)
				for i := 0; i <= x; i++ {
					err := b.Put(fastrand.Bytes(10), fastrand.Bytes(1e3))
					if err != nil {
						t.Errorf("db.Update failed to fill bucket %v for metadata %v, filename %v; error was %v", testBucket, in.md, dbFilename, err)
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Error(err)
			continue
		}
		// Close the newly-filled database.
		err = db.Close()
		if err != nil {
			t.Errorf("closing a newly-filled database failed for metadata %v, filename %v; error was %v", in.md, dbFilename, err)
			continue
		}
		// Call OpenDatabase on the database now that it's been filled.
		db, err = OpenDatabase(in.md, dbFilepath)
		if err != nil {
			t.Error(err)
			continue
		}
		// Empty every bucket in the database.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				b := tx.Bucket(testBucket)
				err := b.ForEach(func(k, v []byte) error {
					return b.Delete(k)
				})
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Error(err)
			continue
		}
		// Close and delete the newly emptied database.
		err = db.Close()
		if err != nil {
			t.Errorf("closing a newly-emptied database for metadata %v, filename %v; error was %v", in.md, dbFilename, err)
			continue
		}
		err = os.Remove(dbFilepath)
		if err != nil {
			t.Errorf("removing database file failed for metadata %v, filename %v; error was %v", in.md, dbFilename, err)
			continue
		}
	}
}

// TestErrPermissionOpenDatabase tests calling OpenDatabase on a database file
// with the wrong filemode (< 0600), which should result in an os.ErrPermission
// error.
func TestErrPermissionOpenDatabase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("can't reproduce on Windows")
	}

	const (
		dbHeader   = "Fake Header"
		dbVersion  = "0.0.0"
		dbFilename = "Fake Filename"
	)
	testDir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	dbFilepath := filepath.Join(testDir, dbFilename)
	badFileModes := []os.FileMode{0000, 0001, 0002, 0003, 0004, 0005, 0010, 0040, 0060, 0105, 0110, 0126, 0130, 0143, 0150, 0166, 0170, 0200, 0313, 0470, 0504, 0560, 0566, 0577}

	// Make sure OpenDatabase returns a permissions error for each of the modes
	// in badFileModes.
	for _, mode := range badFileModes {
		// Create a file named dbFilename in directory testDir with the wrong
		// permissions (mode < 0600).
		_, err := os.OpenFile(dbFilepath, os.O_RDWR|os.O_CREATE, mode)
		if err != nil {
			t.Fatal(err)
		}
		// OpenDatabase should return a permissions error because the database
		// mode is less than 0600.
		_, err = OpenDatabase(Metadata{dbHeader, dbVersion}, dbFilepath)
		if !os.IsPermission(err) {
			t.Errorf("OpenDatabase failed to return expected error when called on a database with the wrong permissions (%o instead of >= 0600);\n wanted:\topen %v: permission denied\n got:\t\t%v", mode, dbFilepath, err)
		}
		err = os.Remove(dbFilepath)
		if err != nil {
			t.Error(err)
		}
	}
}

// TestErrTxNotWritable checks that updateMetadata returns an error when called
// from a read-only transaction.
func TestErrTxNotWritable(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	for i, in := range testInputs {
		dbFilename := testFilenames[i%len(testFilenames)]
		dbFilepath := filepath.Join(testDir, dbFilename)
		db, err := bolt.Open(dbFilepath, 0600, &bolt.Options{Timeout: 3 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		boltDB := &BoltDatabase{
			Metadata: in.md,
			DB:       db,
		}
		// Should return an error because updateMetadata is being called from
		// a read-only transaction.
		err = db.View(boltDB.updateMetadata)
		if err != bolt.ErrTxNotWritable {
			t.Errorf("updateMetadata returned wrong error for input %v, filename %v; expected tx not writable, got %v", in.md, dbFilename, err)
		}
		err = boltDB.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = os.Remove(dbFilepath)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestErrDatabaseNotOpen tests that checkMetadata returns an error when called
// on a BoltDatabase that is closed.
func TestErrDatabaseNotOpen(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	dbFilepath := filepath.Join(testDir, "fake_filename")
	md := Metadata{"Fake Header", "Fake Version"}
	db, err := bolt.Open(dbFilepath, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	boltDB := &BoltDatabase{
		Metadata: md,
		DB:       db,
	}
	err = boltDB.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Should return an error since boltDB is closed.
	err = boltDB.checkMetadata(md)
	if err != bolt.ErrDatabaseNotOpen {
		t.Errorf("expected database not open, got %v", err)
	}
	err = os.Remove(dbFilepath)
	if err != nil {
		t.Error(err)
	}
}

// TestErrCheckMetadata tests that checkMetadata returns an error when called
// on a BoltDatabase whose metadata has been changed.
func TestErrCheckMetadata(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	for i, in := range testInputs {
		dbFilename := testFilenames[i%len(testFilenames)]
		dbFilepath := filepath.Join(testDir, dbFilename)
		db, err := bolt.Open(dbFilepath, 0600, &bolt.Options{Timeout: 3 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		boltDB := &BoltDatabase{
			Metadata: in.md,
			DB:       db,
		}
		err = db.Update(func(tx *bolt.Tx) error {
			bucket, err := tx.CreateBucketIfNotExists([]byte("Metadata"))
			if err != nil {
				return err
			}
			err = bucket.Put([]byte("Header"), []byte(in.newMd.Header))
			if err != nil {
				return err
			}
			err = bucket.Put([]byte("Version"), []byte(in.newMd.Version))
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Errorf("Put method failed for input %v, filename %v with error %v", in, dbFilename, err)
			continue
		}
		// Should return an error because boltDB's metadata now differs from
		// its original metadata.
		err = (*boltDB).checkMetadata(in.md)
		if err != in.err {
			t.Errorf("expected %v, got %v for input %v -> %v", in.err, err, in.md, in.newMd)
		}
		err = boltDB.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = os.Remove(dbFilepath)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestErrIntegratedCheckMetadata checks that checkMetadata returns an error
// within OpenDatabase when OpenDatabase is called on a BoltDatabase that has
// already been set up with different metadata.
func TestErrIntegratedCheckMetadata(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	testDir := build.TempDir(persistDir, t.Name())
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}
	for i, in := range testInputs {
		dbFilename := testFilenames[i%len(testFilenames)]
		dbFilepath := filepath.Join(testDir, dbFilename)
		boltDB, err := OpenDatabase(in.md, dbFilepath)
		if err != nil {
			t.Errorf("OpenDatabase failed on input %v, filename %v; error was %v", in, dbFilename, err)
			continue
		}
		err = boltDB.Close()
		if err != nil {
			t.Fatal(err)
		}
		// Should return an error because boltDB was set up with metadata in.md, not in.newMd
		boltDB, err = OpenDatabase(in.newMd, dbFilepath)
		if err != in.err {
			t.Errorf("expected error %v for input %v and filename %v; got %v instead", in.err, in, dbFilename, err)
		}
		err = os.Remove(dbFilepath)
		if err != nil {
			t.Fatal(err)
		}
	}
}
