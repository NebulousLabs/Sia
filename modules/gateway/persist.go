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

// persistMetadata contains all of the
var persistMetadata = persist.Metadata{
	Header:  "Sia Node List",
	Version: "0.3.3",
}

// load pulls the gateway persistent data off disk and into memory.
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

// save stores the gateway's persistent data on disk.
func (g *Gateway) save() error {
	var nodes []modules.NetAddress
	for node := range g.nodes {
		nodes = append(nodes, node)
	}
	return persist.SaveFile(persistMetadata, nodes, filepath.Join(g.persistDir, nodesFile))
}

// saveSync stores the gateway's persistent data on disk, and then syncs to
// disk to minimize the possibility of data loss.
func (g *Gateway) saveSync() error {
	var nodes []modules.NetAddress
	for node := range g.nodes {
		nodes = append(nodes, node)
	}
	return persist.SaveFileSync(persistMetadata, nodes, filepath.Join(g.persistDir, nodesFile))
}
