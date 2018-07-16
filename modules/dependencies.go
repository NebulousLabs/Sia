package modules

import (
	"errors"
	"io"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"time"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/persist"
	"gitlab.com/NebulousLabs/fastrand"
)

// ProdDependencies act as a global instance of the production dependencies to
// avoid having to instantiate new dependencies every time we want to pass
// production dependencies.
var ProdDependencies = new(ProductionDependencies)

// Dependencies defines dependencies used by all of Sia's modules. Custom
// dependencies can be created to inject certain behavior during testing.
type (
	Dependencies interface {
		// AtLeastOne will return a value that is at least one. In production,
		// the value should always be one. This function is used to test the
		// idempotency of actions, so during testing sometimes the value
		// returned will be higher, causing an idempotent action to be
		// committed multiple times. If the action is truly idempotent,
		// committing it multiple times should not cause any problems or
		// changes.
		AtLeastOne() uint64

		// CreateFile gives the host the ability to create files on the
		// operating system.
		CreateFile(string) (File, error)

		// Destruct will clean up the dependencies, panicking if there are
		// unclosed resources.
		Destruct()

		// DialTimeout tries to create a tcp connection to the specified
		// address with a certain timeout.
		DialTimeout(NetAddress, time.Duration) (net.Conn, error)

		// Disrupt can be inserted in the code as a way to inject problems,
		// such as a network call that take 10 minutes or a disk write that
		// never completes. disrupt will return true if the disruption is
		// forcibly triggered. In production, disrupt will always return false.
		Disrupt(string) bool

		// Listen gives the host the ability to receive incoming connections.
		Listen(string, string) (net.Listener, error)

		// LoadFile allows the host to load a persistence structure form disk.
		LoadFile(persist.Metadata, interface{}, string) error

		// MkdirAll gives the host the ability to create chains of folders
		// within the filesystem.
		MkdirAll(string, os.FileMode) error

		// NewLogger creates a logger that the host can use to log messages and
		// write critical statements.
		NewLogger(string) (*persist.Logger, error)

		// OpenDatabase creates a database that the host can use to interact
		// with large volumes of persistent data.
		OpenDatabase(persist.Metadata, string) (*persist.BoltDatabase, error)

		// OpenFile opens a file for the host.
		OpenFile(string, int, os.FileMode) (File, error)

		// RandRead fills the input bytes with random data.
		RandRead([]byte) (int, error)

		// ReadFile reads a file in full from the filesystem.
		ReadFile(string) ([]byte, error)

		// RemoveFile removes a file from file filesystem.
		RemoveFile(string) error

		// RenameFile renames a file on disk to another name.
		RenameFile(string, string) error

		// SaveFileSync writes JSON encoded data to disk and syncs the file
		// afterwards.
		SaveFileSync(persist.Metadata, interface{}, string) error

		// Sleep blocks the calling thread for at least the specified duration.
		Sleep(time.Duration)

		// Symlink creates a sym link between a source and a destination.
		Symlink(s1, s2 string) error

		// WriteFile writes data to the filesystem using the provided filename.
		WriteFile(string, []byte, os.FileMode) error
	}

	// File implements all of the methods that can be called on an os.File.
	File interface {
		io.ReadWriteCloser
		Name() string
		ReadAt([]byte, int64) (int, error)
		Sync() error
		Truncate(int64) error
		WriteAt([]byte, int64) (int, error)
	}
)

type (
	// ProductionDependencies are the dependencies used in a Release or Debug
	// production build.
	ProductionDependencies struct {
		shouldInit bool
		openFiles  map[string]int
		mu         sync.Mutex
	}

	// ProductionFile is the implementation of the File interface that is used
	// in a Release or Debug production build.
	ProductionFile struct {
		pd *ProductionDependencies
		*os.File
	}
)

// Close will close a file, checking whether the file handle is open somewhere
// else before closing completely. This check is performed on Windows but not
// Linux, therefore a mock is used to ensure that linux testing picks up
// potential problems that would be seen on Windows.
func (pf *ProductionFile) Close() error {
	if !build.DEBUG {
		return pf.File.Close()
	}

	pf.pd.mu.Lock()
	if pf.pd.openFiles == nil {
		pf.pd.openFiles = make(map[string]int)
	}
	v, exists := pf.pd.openFiles[pf.File.Name()]
	if !exists {
		panic("file not registered")
	}
	if v == 1 {
		delete(pf.pd.openFiles, pf.File.Name())
	} else if v > 1 {
		pf.pd.openFiles[pf.File.Name()] = v - 1
	} else {
		panic("inconsistent state")
	}
	pf.pd.mu.Unlock()
	return pf.File.Close()
}

// AtLeastOne will return a value that is equal to 1 if debugging is disabled.
// If debugging is enabled, a higher value may be returned.
func (*ProductionDependencies) AtLeastOne() uint64 {
	if !build.DEBUG {
		return 1
	}

	// Probabilistically return a number greater than one.
	val := uint64(1)
	for fastrand.Intn(2) != 0 {
		val++
	}
	return val
}

// CreateFile gives the host the ability to create files on the operating
// system.
func (pd *ProductionDependencies) CreateFile(s string) (File, error) {
	if !build.DEBUG {
		return os.Create(s)
	}

	f, err := os.Create(s)
	if err != nil {
		return f, err
	}

	pd.mu.Lock()
	if pd.openFiles == nil {
		pd.openFiles = make(map[string]int)
	}
	v := pd.openFiles[s]
	pd.openFiles[s] = v + 1
	pd.mu.Unlock()
	return &ProductionFile{
		pd:   pd,
		File: f,
	}, nil
}

// Destruct checks that all resources have been cleaned up correctly.
func (pd *ProductionDependencies) Destruct() {
	if !build.DEBUG {
		return
	}

	pd.mu.Lock()
	l := len(pd.openFiles)
	pd.mu.Unlock()
	if l != 0 {
		panic("unclosed resources - most likely file handles")
	}
}

// DialTimeout creates a tcp connection to a certain address with the specified
// timeout.
func (*ProductionDependencies) DialTimeout(addr NetAddress, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", string(addr), timeout)
}

// Disrupt can be used to inject specific behavior into a module by overwriting
// it using a custom dependency.
func (*ProductionDependencies) Disrupt(string) bool {
	return false
}

// Listen gives the host the ability to receive incoming connections.
func (*ProductionDependencies) Listen(s1, s2 string) (net.Listener, error) {
	return net.Listen(s1, s2)
}

// LoadFile loads JSON encoded data from a file.
func (*ProductionDependencies) LoadFile(meta persist.Metadata, data interface{}, filename string) error {
	return persist.LoadJSON(meta, data, filename)
}

// SaveFileSync writes JSON encoded data to a file and syncs the file to disk
// afterwards.
func (*ProductionDependencies) SaveFileSync(meta persist.Metadata, data interface{}, filename string) error {
	return persist.SaveJSON(meta, data, filename)
}

// MkdirAll gives the host the ability to create chains of folders within the
// filesystem.
func (*ProductionDependencies) MkdirAll(s string, fm os.FileMode) error {
	return os.MkdirAll(s, fm)
}

// NewLogger creates a logger that the host can use to log messages and write
// critical statements.
func (*ProductionDependencies) NewLogger(s string) (*persist.Logger, error) {
	return persist.NewFileLogger(s)
}

// OpenDatabase creates a database that the host can use to interact with large
// volumes of persistent data.
func (*ProductionDependencies) OpenDatabase(m persist.Metadata, s string) (*persist.BoltDatabase, error) {
	return persist.OpenDatabase(m, s)
}

// OpenFile opens a file for the contract manager.
func (pd *ProductionDependencies) OpenFile(s string, i int, fm os.FileMode) (File, error) {
	if !build.DEBUG {
		return os.OpenFile(s, i, fm)
	}

	f, err := os.OpenFile(s, i, fm)
	if err != nil {
		return f, err
	}

	pd.mu.Lock()
	if pd.openFiles == nil {
		pd.openFiles = make(map[string]int)
	}
	v := pd.openFiles[s]
	pd.openFiles[s] = v + 1
	pd.mu.Unlock()
	return &ProductionFile{
		pd:   pd,
		File: f,
	}, nil
}

// RandRead fills the input bytes with random data.
func (*ProductionDependencies) RandRead(b []byte) (int, error) {
	return fastrand.Reader.Read(b)
}

// ReadFile reads a file from the filesystem.
func (*ProductionDependencies) ReadFile(s string) ([]byte, error) {
	return ioutil.ReadFile(s)
}

// RemoveFile will remove a file from disk.
func (pd *ProductionDependencies) RemoveFile(s string) error {
	if !build.DEBUG {
		return os.Remove(s)
	}

	pd.mu.Lock()
	if pd.openFiles == nil {
		pd.openFiles = make(map[string]int)
	}
	v, exists := pd.openFiles[s]
	pd.mu.Unlock()
	if exists && v > 0 {
		return errors.New("cannot remove the file, it's open somewhere else right now")
	}
	return os.Remove(s)
}

// RenameFile renames a file on disk.
func (pd *ProductionDependencies) RenameFile(s1 string, s2 string) error {
	if !build.DEBUG {
		return os.Rename(s1, s2)
	}

	pd.mu.Lock()
	if pd.openFiles == nil {
		pd.openFiles = make(map[string]int)
	}
	v1, exists1 := pd.openFiles[s1]
	v2, exists2 := pd.openFiles[s2]
	pd.mu.Unlock()
	if exists1 && v1 > 0 {
		return errors.New("cannot remove the file, it's open somewhere else right now")
	}
	if exists2 && v2 > 0 {
		return errors.New("cannot remove the file, it's open somewhere else right now")
	}
	return os.Rename(s1, s2)
}

// Sleep blocks the calling thread for a certain duration.
func (*ProductionDependencies) Sleep(d time.Duration) {
	time.Sleep(d)
}

// Symlink creates a symlink between a source and a destination file.
func (*ProductionDependencies) Symlink(s1, s2 string) error {
	return os.Symlink(s1, s2)
}

// WriteFile writes a file to the filesystem.
func (*ProductionDependencies) WriteFile(s string, b []byte, fm os.FileMode) error {
	return ioutil.WriteFile(s, b, fm)
}
