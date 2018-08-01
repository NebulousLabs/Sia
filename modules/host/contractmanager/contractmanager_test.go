package contractmanager

import (
	"errors"
	"os"
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
func newMockedContractManagerTester(d modules.Dependencies, name string) (*contractManagerTester, error) {
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
	t.Parallel()

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

// dependencyErroredStartupis a mocked dependency that will cause the contract
// manager to be returned with an error upon startup.
type dependencyErroredStartup struct {
	modules.ProductionDependencies
}

// disrupt will disrupt the threadedSyncLoop, causing the loop to terminate as
// soon as it is created.
func (d *dependencyErroredStartup) Disrupt(s string) bool {
	// Cause an error to be returned during startup.
	if s == "erroredStartup" {
		return true
	}
	return false
}

// TestNewContractManagerErroredStartup uses disruption to simulate an error
// during startup, allowing the test to verify that the cleanup code ran
// correctly.
func TestNewContractManagerErroredStartup(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a new contract manager where the startup gets disrupted.
	d := new(dependencyErroredStartup)
	testdir := build.TempDir(modules.ContractManagerDir, "TestNewContractManagerErroredStartup")
	cmd := filepath.Join(testdir, modules.ContractManagerDir)
	_, err := newContractManager(d, cmd)
	if err == nil || err.Error() != "startup disrupted" {
		t.Fatal("expecting contract manager startup to be disrupted:", err)
	}

	// Verify that shutdown was triggered correctly - tmp files should be gone,
	// WAL file should also be gone.
	walFileName := filepath.Join(cmd, walFile)
	walFileTmpName := filepath.Join(cmd, walFileTmp)
	settingsFileTmpName := filepath.Join(cmd, settingsFileTmp)
	_, err = os.Stat(walFileName)
	if !os.IsNotExist(err) {
		t.Error("file should have been removed:", err)
	}
	_, err = os.Stat(walFileTmpName)
	if !os.IsNotExist(err) {
		t.Error("file should have been removed:", err)
	}
	_, err = os.Stat(settingsFileTmpName)
	if !os.IsNotExist(err) {
		t.Error("file should have been removed:", err)
	}
}
