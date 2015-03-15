package tester

import (
	"path/filepath"
	"strconv"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/gateway"
)

// CreateTestingGateway creates a ready-to-use gateway.
func CreateTestingGateway(directory string) (g modules.Gateway, err error) {
	state, err := CreateTestingConsensusSet(directory)
	if err != nil {
		return
	}

	port := NewPort()
	strPort := ":" + strconv.Itoa(port)
	gDir := filepath.Join(TempDir(directory), modules.GatewayDir)
	g, err = gateway.New(strPort, state, gDir)
	return
}
