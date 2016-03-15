package gateway

import (
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	// nodesFile is the name of the file that contains all seen nodes.
	nodesFile = "nodes.json"
	// logFile is the name of the log file.
	logFile = modules.GatewayDir + ".log"
)

var persistMetadata = persist.Metadata{
	Header:  "Sia Node List",
	Version: "0.3.3",
}

func (g *Gateway) save() error {
	var nodes []modules.NetAddress
	for node := range g.nodes {
		nodes = append(nodes, node)
	}
	return persist.SaveFile(persistMetadata, nodes, filepath.Join(g.persistDir, nodesFile))
}

func (g *Gateway) load() error {
	var nodes []modules.NetAddress
	err := persist.LoadFile(persistMetadata, &nodes, filepath.Join(g.persistDir, nodesFile))
	if err != nil {
		return err
	}
	for _, node := range nodes {
		g.addNode(node)
	}
	return nil
}
