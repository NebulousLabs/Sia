package host

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

// dependencyErrMkdirAll is a dependency set that returns an error when MkdirAll
// is called.
type dependencyErrMkdirAll struct {
	modules.ProductionDependencies
}

func (*dependencyErrMkdirAll) MkdirAll(string, os.FileMode) error {
	return mockErrMkdirAll
}

// TestHostFailedMkdirAll initializes the host using a call to MkdirAll that
// will fail.
func TestHostFailedMkdirAll(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestHostFailedMkdirAll")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = newHost(&dependencyErrMkdirAll{}, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != mockErrMkdirAll {
		t.Fatal(err)
	}
	// Set ht.host to something non-nil - nil was returned because startup was
	// incomplete. If ht.host is nil at the end of the function, the ht.Close()
	// operation will fail.
	ht.host, err = newHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
}

// dependencyErrNewLogger is a dependency set that returns an error when
// NewLogger is called.
type dependencyErrNewLogger struct {
	modules.ProductionDependencies
}

func (*dependencyErrNewLogger) NewLogger(string) (*persist.Logger, error) {
	return nil, mockErrNewLogger
}

// TestHostFailedNewLogger initializes the host using a call to NewLogger that
// will fail.
func TestHostFailedNewLogger(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestHostFailedNewLogger")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = newHost(&dependencyErrNewLogger{}, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != mockErrNewLogger {
		t.Fatal(err)
	}
	// Set ht.host to something non-nil - nil was returned because startup was
	// incomplete. If ht.host is nil at the end of the function, the ht.Close()
	// operation will fail.
	ht.host, err = newHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
}

// dependencyErrOpenDatabase is a dependency that returns an error when
// OpenDatabase is called.
type dependencyErrOpenDatabase struct {
	modules.ProductionDependencies
}

func (*dependencyErrOpenDatabase) OpenDatabase(persist.Metadata, string) (*persist.BoltDatabase, error) {
	return nil, mockErrOpenDatabase
}

// TestHostFailedOpenDatabase initializes the host using a call to OpenDatabase
// that has been mocked to fail.
func TestHostFailedOpenDatabase(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestHostFailedOpenDatabase")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = newHost(&dependencyErrOpenDatabase{}, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err == nil || !strings.Contains(err.Error(), "simulated OpenDatabase failure") {
		t.Fatal("Opening database should have failed", err)
	}
	// Set ht.host to something non-nil - nil was returned because startup was
	// incomplete. If ht.host is nil at the end of the function, the ht.Close()
	// operation will fail.
	ht.host, err = newHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
}

// dependencyErrLoadFile is a dependency that returns an error when
// LoadFile is called.
type dependencyErrLoadFile struct {
	modules.ProductionDependencies
}

func (*dependencyErrLoadFile) LoadFile(persist.Metadata, interface{}, string) error {
	return mockErrLoadFile
}

// TestHostFailedLoadFile initializes the host using a call to LoadFile that
// has been mocked to fail.
func TestHostFailedLoadFile(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestHostFailedLoadFile")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = newHost(&dependencyErrLoadFile{}, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != mockErrLoadFile {
		t.Fatal(err)
	}
	// Set ht.host to something non-nil - nil was returned because startup was
	// incomplete. If ht.host is nil at the end of the function, the ht.Close()
	// operation will fail.
	ht.host, err = newHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
}

// dependencyErrListen is a dependency that returns an error when Listen is
// called.
type dependencyErrListen struct {
	modules.ProductionDependencies
}

func (*dependencyErrListen) Listen(string, string) (net.Listener, error) {
	return nil, mockErrListen
}

// TestHostFailedListen initializes the host using a call to Listen that
// has been mocked to fail.
func TestHostFailedListen(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestHostFailedListen")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	ht.host, err = newHost(&dependencyErrListen{}, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != mockErrListen {
		t.Fatal(err)
	}
	// Set ht.host to something non-nil - nil was returned because startup was
	// incomplete. If ht.host is nil at the end of the function, the ht.Close()
	// operation will fail.
	ht.host, err = newHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
}
