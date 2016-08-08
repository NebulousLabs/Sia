package contractmanager

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
)

// contractManagerTester holds a contract manager along with some other fields
// useful for testing, and has methods implemented on it that can assist
// testing.
type contractManagerTester struct {
	cm *ContractManager

	persistDir string
}

// panicClose will attempt to call Close on the contract manager tester. If
// there is an error, the function will panic. A convenient function for making
// sure that the cleanup code is always running correctly, without needing to
// write a lot of boiler code.
func (cmt *contractManagerTester) panicClose() {
	err := cmt.Close()
	if err != nil {
		panic(err)
	}
}

// Close will perform clean shutdown on the contract manager tester.
func (cmt *contractManagerTester) Close() error {
	if cmt.cm == nil {
		return errors.New("nil contract manager")
	}
	return cmt.cm.Close()
}

// newContractManagerTester returns a ready-to-rock contract manager tester.
func newContractManagerTester(name string) (*contractManagerTester, error) {
	if testing.Short() {
		panic("use of newContractManagerTester during short testing")
	}

	testdir := build.TempDir(modules.ContractManagerDir, name)
	cm, err := New(filepath.Join(testdir, modules.ContractManagerDir))
	if err != nil {
		return nil, err
	}
	cmt := &contractManagerTester{
		cm:         cm,
		persistDir: testdir,
	}
	return cmt, nil
}

// newMockedContractManagerTester returns a contract manager tester that uses
// the input dependencies instead of the production ones.
func newMockedContractManagerTester(d dependencies, name string) (*contractManagerTester, error) {
	if testing.Short() {
		panic("use of newContractManagerTester during short testing")
	}

	testdir := build.TempDir(modules.ContractManagerDir, name)
	cm, err := newContractManager(d, filepath.Join(testdir, modules.ContractManagerDir))
	if err != nil {
		return nil, err
	}
	cmt := &contractManagerTester{
		cm:         cm,
		persistDir: testdir,
	}
	return cmt, nil
}

// TestNewContractManager does basic startup and shutdown of a contract
// manager, checking for egregious errors.
func TestNewContractManager(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create a contract manager.
	parentDir := build.TempDir(modules.ContractManagerDir, "TestNewContractManager")
	cmDir := filepath.Join(parentDir, modules.ContractManagerDir)
	cm, err := New(cmDir)
	if err != nil {
		t.Fatal(err)
	}
	// Close the contract manager.
	err = cm.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Create a new contract manager using the same directory.
	cm, err = New(cmDir)
	if err != nil {
		t.Fatal(err)
	}
	// Close it again.
	err = cm.Close()
	if err != nil {
		t.Fatal(err)
	}
}
