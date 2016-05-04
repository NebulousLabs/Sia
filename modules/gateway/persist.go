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

// persistMetadata contains the header and version strings that identify the
// gateway persist file.
var persistMetadata = persist.Metadata{
	Header:  "Sia Node List",
	Version: "0.3.3",
}

// persistData returns the data in the Gateway that will be saved to disk.
func (g *Gateway) persistData() (nodes []modules.NetAddress) {
	for node := range g.nodes {
		nodes = append(nodes, node)
	}
	return
}

// load loads the Gateway's persistent data from disk.
func (g *Gateway) load() error {
	var nodes []modules.NetAddress
	err := persist.LoadFile(persistMetadata, &nodes, filepath.Join(g.persistDir, nodesFile))
	if err != nil {
		return err
	}
	for _, node := range nodes {
		err := g.addNode(node)
		if err != nil {
			g.log.Printf("WARN: error loading node '%v' from persist: %v", node, err)
		}
	}
	return nil
}

// save stores the Gateway's persistent data on disk.
func (g *Gateway) save() error {
	return persist.SaveFile(persistMetadata, g.persistData(), filepath.Join(g.persistDir, nodesFile))
}

// saveSync stores the Gateway's persistent data on disk, and then syncs to
// disk to minimize the possibility of data loss.
func (g *Gateway) saveSync() error {
	return persist.SaveFileSync(persistMetadata, g.persistData(), filepath.Join(g.persistDir, nodesFile))
}
