// Borrowed weird strings from https://github.com/minimaxir/big-list-of-naughty-strings

package persist

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/bolt"
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

	testInputs := []struct {
		dbMetadata Metadata
		dbFilename string
	}{
		{Metadata{"", ""}, " "},
		{Metadata{"", ""}, "_"},
		{Metadata{"_", "_"}, "_"},
		{Metadata{"asdf", "asdf"}, "asdf"},
		{Metadata{"1sadf23", "12253"}, "123kjhgfd"},
		{Metadata{"$@#$%^&", "$@#$%^&"}, "$@#$%^&"},
		{Metadata{"//", "//"}, "_"},
		{Metadata{"testHeader" + RandomSuffix(), "0.0.0"}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader	" + RandomSuffix(), "7.0.4"}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader?" + RandomSuffix(), "asdf"}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader...." + RandomSuffix(), ""}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader/asdf" + RandomSuffix(), "_"}, "testFilename" + RandomSuffix()},
		{Metadata{":]", ":)"}, ":|"},
		{Metadata{"¯|_(ツ)_|¯","_|¯(ツ)¯|_"}, "¯|_(ツ)_|¯"},
		{Metadata{"世界", "怎么办呢"}, "你好好好"},
		{Metadata{"		","		"}," "},
		{Metadata{"你好		好 好", "好a好3好你"}, "你好好q wgc好"},
		{Metadata{"apparently \xF0\x9F\x98\x8F","\xF0\x9F\x98\xBE"}, "\xF0\x9F\x99\x8A"},
		{Metadata{"\xF0\x9F\x98\x8F","\xF0\x9F\x98\xBE	emoji"}, "\xF0\x9F\x99\x8A"},
		{Metadata{"\xF0\x9F\x98\x8F","\xF0\x9F\x98\xBE"}, "are okay?\xF0\x9F\x99\x8A"},
		{Metadata{"nil","undefined"}, "A:"},		
		{Metadata{"⒯⒣⒠ ⒬⒰⒤⒞⒦ ⒝⒭⒪⒲⒩ ⒡⒪⒳ ⒥⒰⒨⒫⒮ ⒪⒱⒠⒭ ⒯⒣⒠ ⒧⒜⒵⒴ ⒟⒪⒢","undefined"}, "PRN"},		
		{Metadata{"\n","Ṱ̺̺̕o͞ ̷i̲̬͇̪͙n̝̗͕v̟̜̘̦͟o̶̙̰̠kè͚̮̺̪̹̱̤ ̖t̝͕̳̣̻̪͞h̼͓̲̦̳̘̲e͇̣̰̦̬͎ ̢̼̻̱̘h͚͎͙̜̣̲ͅi̦̲̣̰̤v̻͍e̺̭̳̪̰-m̢iͅn̖̺̞̲̯̰d̵̼̟͙̩̼̘̳ ̞̥̱̳̭r̛̗̘e͙p͠r̼̞̻̭̗e̺̠̣͟s̘͇̳͍̝͉e͉̥̯̞̲͚̬͜ǹ̬͎͎̟̖͇̤t͍̬̤͓̼̭͘ͅi̪̱n͠g̴͉ ͏͉ͅc̬̟h͡a̫̻̯͘o̫̟̖͍̙̝͉s̗̦̲.̨̹͈̣"}, "CON"},		
		{Metadata{"𝕋𝕙𝕖 𝕢𝕦𝕚𝕔𝕜 𝕓𝕣𝕠𝕨𝕟 𝕗𝕠𝕩 𝕛𝕦𝕞𝕡𝕤 𝕠𝕧𝕖𝕣 𝕥𝕙𝕖 𝕝𝕒𝕫𝕪 𝕕𝕠𝕘","test"}, "␣"},		
		{Metadata{"⁰⁴⁵₀₁₂","⅛⅜⅝⅞"}, " "},		
		{Metadata{"הָיְתָהtestالصفحات التّحول",  "مُنَاقَشَةُ سُبُلِ اِسْتِخْدَامِ اللُّغَةِ فِي النُّظُمِ الْقَائِمَةِ وَفِيم يَخُصَّ التَّطْبِيقَاتُ الْحاسُوبِيَّةُ،"},"$HOME"},		
		{Metadata{"<foo val=“bar” />","(ﾉಥ益ಥ ┻━┻"}, "$HOME"},		
		{Metadata{"!@#$%^&*()`~","<>?:\"{}|_+/"}, ",.;'[]-="},		
		{Metadata{"true","false"}, "A:"},		
		{Metadata{"Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗","Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗"}, "Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗"},		
		{Metadata{"%*.*s","%d"}, "%s"},		
	}
	
	testBuckets := [][]byte{
		[]byte("FakeBucket"),
		[]byte("FakeBucket123"),
		[]byte("FakeBucket123!@#$"),
		[]byte("Another Fake Bucket"),
		[]byte("FakeBucket" + RandomSuffix()),
		[]byte("_"),
		[]byte(" asdf"),
		[]byte("你好好好"),
		[]byte("¯|_(ツ)_|¯"),
		[]byte("Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗"),
		[]byte("﷽"),
		[]byte("(ﾉಥ益ಥ ┻━┻"),
		[]byte("Ṱ̺̺o͞ ̷i̲̬n̝̗v̟̜o̶̙kè͚̮ ̖t̝͕h̼͓e͇̣ ̢̼h͚͎i̦̲v̻͍e̺̭-m̢iͅn̖̺d̵̼ ̞̥r̛̗e͙p͠r̼̞e̺̠s̘͇e͉̥ǹ̬͎t͍̬i̪̱n͠g̴͉ ͏͉c̬̟h͡a̫̻o̫̟s̗̦.̨̹"),
		[]byte("0xbadidea"),
		[]byte("nil"),
		[]byte("你好好好"),
	}

	// Create a folder for the database file. If a folder by that name exists
	// already, it will be replaced by an empty folder.
	testDir := build.TempDir(persistDir, "TestOpenNewDatabase")
		err := os.MkdirAll(testDir, 0700)
		if err != nil {
			t.Fatal(err)
		}

	// Loop through tests for each testInput.
	for _, in := range testInputs {
		dbFilePath := filepath.Join(testDir, in.dbFilename)

		// Create a new database.
		db, err := OpenDatabase(in.dbMetadata, dbFilePath)
		if err != nil {
			t.Fatalf("calling OpenDatabase on a new database failed for input %v; error was %v", in, err)
		}

		// Close the newly-created, empty database.
		err = db.Close()
		if err != nil {
			t.Fatalf("closing a newly created database failed for input %v; error was %v", in, err)
		}

		// Call OpenDatabase again, this time on the existing empty database.
		db, err = OpenDatabase(in.dbMetadata, dbFilePath)
		if err != nil {
			t.Fatalf("calling OpenDatabase on an existing empty database failed for input %v; error was %v", in, err)
		}

		// Create buckets in the database.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				_, err := tx.CreateBucketIfNotExists(testBucket)
				if err != nil {
					t.Fatalf("db.Update failed on bucket name %v; error was", testBucket, err)
					return err
				}
			}
			return nil
		})
		if err != nil {
		}

		// Make sure CreateBucketIfNotExists method handles invalid (nil)
		// bucket name.
		err = db.Update(func(tx *bolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists(nil)
			return err				
		})
		if err != bolt.ErrBucketNameRequired {
			t.Fatalf("the CreateBucketIfNotExists method failed to throw the expected error when fed an invalid (nil) byteslice; expected %v, got %v", bolt.ErrBucketNameRequired, err)
		}

		// Fill each bucket with a random number (0-9, inclusive) of key/value
		// pairs, where each key is a length-10 random byteslice and each value
		// is a length-1000 random byteslice.
		err = db.Update(func(tx *bolt.Tx) error {
			for _, testBucket := range testBuckets {
				b := tx.Bucket(testBucket)
				x := rand.Intn(10)
				for i := 0; i <= x; i++ {
					k := make([]byte, 10)
					rand.Read(k)
					v := make([]byte, 1e3)
					rand.Read(v)
					err := b.Put(k, v)
					if err != nil {
						return err
					}
				}	
			}
		return nil
		})		
		if err != nil {
			t.Fatal(err)
		}	

		// Close the newly-filled database.
		err = db.Close()
		if err != nil {
			t.Fatalf("closing a newly-filled database failed for input %v; error was %v", in, err)
		}

		// Call OpenDatabase on the database now that it's been filled.
		db, err = OpenDatabase(in.dbMetadata, dbFilePath)
		if err != nil {
			t.Fatal(err)
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

		// Close the newly emptied database.
		err = db.Close()
		if err != nil {
			t.Fatalf("closing a newly-emptied database failed for input %v; error was %v", in, err)
		}

		// Clean up by deleting the testfile.
		err = os.Remove(dbFilePath)
		if err != nil {
			t.Fatalf("removing database file failed for input %v; error was %v", in, err)
		}
	}
}

// TestErrPermissionOpenDatabase tests calling OpenDatabase on a database file
// with the wrong filemode (< 0600), which should result in an os.ErrPermission
// error.
func TestErrPermissionOpenDatabase(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	const (
		dbHeader   = "Fake Header"
		dbVersion  = "0.0.0"
		dbFilename = "Fake Filename"
	)

	// Create a folder for the database file. If a folder by that
	// name exists already, it will be replaced by an empty folder.
	testDir := build.TempDir(persistDir, "TestErrPermissionOpenDatabase")
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

// TestErrCheckMetadata tests that checkMetadata returns an error
// when called on a BoltDatabase whose metadata has been changed.
func TestErrCheckMetadata(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	testDir := build.TempDir(persistDir, "TestErrCheckMetadata")
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	dbFilepath := filepath.Join(testDir, "fake_filename")

	testInputs := []struct{
		old		Metadata
		new		Metadata
		err		error
	}{
		{Metadata{"",""}, Metadata{"asdf",""}, ErrBadHeader},
		{Metadata{"",""}, Metadata{"","asdf"}, ErrBadVersion},
		{Metadata{"_",""}, Metadata{"",""}, ErrBadHeader},
		{Metadata{"","_"}, Metadata{"",""}, ErrBadVersion},
		{Metadata{"%&*","#@$"}, Metadata{"","#@$"}, ErrBadHeader},
		{Metadata{"bleep","bloop"}, Metadata{"bloop","bloop"}, ErrBadHeader},
		{Metadata{"blip","blop"}, Metadata{"blip","blip"}, ErrBadVersion},
		{Metadata{"a.sdf","0.30.2"}, Metadata{"a.sdf", "0.3.02" }, ErrBadVersion},
		{Metadata{".asdf","0.30.2"}, Metadata{"asdf.", "0.3.02" }, ErrBadHeader},
		{Metadata{".","0.0.0"}, Metadata{"..","0.0.0"}, ErrBadHeader},
		{Metadata{"haggis","."}, Metadata{"haggis",""}, ErrBadVersion},
		{Metadata{"¯|_(ツ)_|¯",""}, Metadata{"¯|_(ツ)_|¯","¯|_(ツ)_|¯"}, ErrBadVersion},
		{Metadata{",,,,,","2^31"}, Metadata{",,,,","2^31"}, ErrBadHeader},
		{Metadata{"/","/"}, Metadata{"//","/"}, ErrBadHeader},
		{Metadata{" ",""}, Metadata{"	",""}, ErrBadHeader},
		{Metadata{"Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗","Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗"}, Metadata{"Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗","Powerلُلُصّبُلُلصّبُررً ॣ ॣ  ॣ ॣ冗"}, ErrBadVersion},
	}
		
	for _, in := range testInputs {		
		db, err := bolt.Open(dbFilepath, 0600, &bolt.Options{Timeout: 3 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		
		boltDB := &BoltDatabase{
			Metadata: 	in.old,
			DB: 		db,
		}

		err = db.Update(func(tx *bolt.Tx) error {
			bucket, err := tx.CreateBucketIfNotExists([]byte("Metadata"))
			if err != nil {
				return err
			}

			err = bucket.Put([]byte("Header"), []byte(in.new.Header))
			if err != nil {
				return err
			}
			
			err = bucket.Put([]byte("Version"), []byte(in.new.Version))
			if err != nil {
				return err
				}
			return nil
		})

		if err != nil {
			t.Errorf("Put method failed for input %v with error %v", in, err)
			continue
		}	

	
		// checkMetadata should return an error because boltDB's
		// metadata now differs from its original metadata. 
		err = (*boltDB).checkMetadata(in.old) 
		if err != in.err { 
			t.Errorf("expected %v, got %v for input %v -> %v", in.err, err, in.old, in.new)	
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

// TestErrTxNotWritable checks that updateMetadata returns an error
// when called from a read-only transaction.
func TestErrTxNotWritable(t *testing.T) {
	testDir := build.TempDir(persistDir, "TestErrTxNotWritable")
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	testInputs := []struct{
		md			Metadata
		filename	string
	}{
		{Metadata{"", ""}, " "},
		{Metadata{"", ""}, "_"},
		{Metadata{"_", "_"}, "_"},
		{Metadata{"asdf", "asdf"}, "asdf"},
		{Metadata{"1sadf23", "12253"}, "123kjhgfd"},
		{Metadata{"$@#$%^&", "$@#$%^&"}, "$@#$%^&"},
		{Metadata{"//", "//"}, "_"},
		{Metadata{"testHeader" + RandomSuffix(), "0.0.0"}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader	" + RandomSuffix(), "7.0.4"}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader?" + RandomSuffix(), "asdf"}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader...." + RandomSuffix(), ""}, "testFilename" + RandomSuffix()},
		{Metadata{"testHeader/asdf" + RandomSuffix(), "_"}, "testFilename" + RandomSuffix()},
		{Metadata{":]", ":)"}, ":|"},
		{Metadata{"¯|_(ツ)_|¯","_|¯(ツ)¯|_"}, "¯|_(ツ)_|¯"},
		{Metadata{"世界", "怎么办呢"}, "你好好好"},
		{Metadata{"		","		"}," "},
		{Metadata{"你好		好 好", "好a好3好你"}, "你好好q wgc好"},
		{Metadata{"apparently \xF0\x9F\x98\x8F","\xF0\x9F\x98\xBE"}, "\xF0\x9F\x99\x8A"},
		{Metadata{"\xF0\x9F\x98\x8F","\xF0\x9F\x98\xBE	emoji"}, "\xF0\x9F\x99\x8A"},
		{Metadata{"\xF0\x9F\x98\x8F","\xF0\x9F\x98\xBE"}, "are okay?\xF0\x9F\x99\x8A"},
		{Metadata{"nil","undefined"}, "A:"},		
		{Metadata{"⒯⒣⒠ ⒬⒰⒤⒞⒦ ⒝⒭⒪⒲⒩ ⒡⒪⒳ ⒥⒰⒨⒫⒮ ⒪⒱⒠⒭ ⒯⒣⒠ ⒧⒜⒵⒴ ⒟⒪⒢","undefined"}, "PRN"},		
		{Metadata{"\n","Ṱ̺̺̕o͞ ̷i̲̬͇̪͙n̝̗͕v̟̜̘̦͟o̶̙̰̠kè͚̮̺̪̹̱̤ ̖t̝͕̳̣̻̪͞h̼͓̲̦̳̘̲e͇̣̰̦̬͎ ̢̼̻̱̘h͚͎͙̜̣̲ͅi̦̲̣̰̤v̻͍e̺̭̳̪̰-m̢iͅn̖̺̞̲̯̰d̵̼̟͙̩̼̘̳ ̞̥̱̳̭r̛̗̘e͙p͠r̼̞̻̭̗e̺̠̣͟s̘͇̳͍̝͉e͉̥̯̞̲͚̬͜ǹ̬͎͎̟̖͇̤t͍̬̤͓̼̭͘ͅi̪̱n͠g̴͉ ͏͉ͅc̬̟h͡a̫̻̯͘o̫̟̖͍̙̝͉s̗̦̲.̨̹͈̣"}, "CON"},		
		{Metadata{"𝕋𝕙𝕖 𝕢𝕦𝕚𝕔𝕜 𝕓𝕣𝕠𝕨𝕟 𝕗𝕠𝕩 𝕛𝕦𝕞𝕡𝕤 𝕠𝕧𝕖𝕣 𝕥𝕙𝕖 𝕝𝕒𝕫𝕪 𝕕𝕠𝕘","test"}, "␣"},		
		{Metadata{"⁰⁴⁵₀₁₂","⅛⅜⅝⅞"}, " "},		
		{Metadata{"הָיְתָהtestالصفحات التّحول",  "مُنَاقَشَةُ سُبُلِ اِسْتِخْدَامِ اللُّغَةِ فِي النُّظُمِ الْقَائِمَةِ وَفِيم يَخُصَّ التَّطْبِيقَاتُ الْحاسُوبِيَّةُ،"},"$HOME"},		
		{Metadata{"<foo val=“bar” />","(ﾉಥ益ಥ ┻━┻"}, "$HOME"},		
		{Metadata{"!@#$%^&*()`~","<>?:\"{}|_+/"}, ",.;'[]-="},		
		{Metadata{"true","false"}, "A:"},		
		{Metadata{"Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗","Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗"}, "Powerلُلُصّبُلُلصّبُررً ॣ ॣh ॣ ॣ冗"},		
		{Metadata{"%*.*s","%d"}, "%s"},		

	}

	for _, in := range testInputs {

		dbFilepath := filepath.Join(testDir, in.filename)

		db, err := bolt.Open(dbFilepath, 0600, &bolt.Options{Timeout: 3 * time.Second})
		if err != nil {
			t.Fatal(err)
		}

		boltDB := &BoltDatabase{
			Metadata: in.md,
			DB: db,
		}

		tx, err := db.Begin(false)
		// Should return an error since tx is a read-only transaction.
		err = boltDB.updateMetadata(tx)
		if err != bolt.ErrTxNotWritable {
			t.Errorf("expected tx not writable, got %v", err)
		}

		tx.Commit()
		boltDB.Close()
		err = os.Remove(dbFilepath)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestErrDatabaseNotOpen tests that checkMetadata returns an error
// when called on a BoltDatabase that is closed.
func TestErrDatabaseNotOpen(t *testing.T) {
	testDir := build.TempDir(persistDir, "TestErrDatabaseNotOpen")
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	dbFilepath := filepath.Join(testDir, "fake_filename")
	md := Metadata{"Fake Header","Fake Version"}

	db, err := bolt.Open(dbFilepath, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}

	boltDB := &BoltDatabase{
		Metadata: md,
		DB: db,
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


// TestErrIntegratedCheckMetadata checks that checkMetadata returns an error
// within OpenDatabase when OpenDatabase is called on a BoltDatabase that 
// has already been set up with different metadata.
func TestErrIntegratedCheckMetadata(t *testing.T) {
	testDir := build.TempDir(persistDir, "TestErrCheckMetadata")
	err := os.MkdirAll(testDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	dbFilepath := filepath.Join(testDir, "fake_filename")
	old := Metadata{"Old Header", "Old Version"}
	new := Metadata{"New Header", "New Version"}
	testErr := ErrBadHeader

	boltDB, err := OpenDatabase(old, dbFilepath)
	if err != nil {
		t.Fatal(err)
	}

	err = boltDB.Close()
	if err != nil {
		t.Fatal(err)
	}

	boltDB, err = OpenDatabase(new, dbFilepath)
	if err != testErr {
		t.Error("expected error %v for input %v -> %v, got %v instead", testErr, old, new, err)
	}
}
