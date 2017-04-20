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
	Version: "0.4.0",
}

// previousPersistMetadata contains the header and version strings that
// identify the previous gateway persist file. It allows us to load files in a
// backward-compatible way.
var previousPersistMetadata = persist.Metadata{
	Header:  "Sia Node List",
	Version: "0.3.3",
}

type nodePersist struct {
	NetAddress modules.NetAddress `json:"netaddress"`
	Inbound    bool               `json:"inbound"`
}

type gatewayPersist struct {
	Nodes []nodePersist `json:"nodes"`
}

// TODO: ensure that we actually have all outbound nodes,
//       as it seems that peers disconnect

// persistData returns the data in the Gateway that will be saved to disk.
func (g *Gateway) persistData() (data gatewayPersist) {
	for address, node := range g.nodes {
		data.Nodes = append(data.Nodes, nodePersist{
			NetAddress: address,
			Inbound:    node.Inbound,
		})
	}

	return
}

// load loads the Gateway's persistent data from disk.
func (g *Gateway) load() error {
	var data gatewayPersist

	persistFile := filepath.Join(g.persistDir, nodesFile)
	err := persist.LoadFile(persistMetadata, &data, persistFile)
	if err == persist.ErrBadVersion {
		// might be a file from the previous version
		var addresses []modules.NetAddress
		err = persist.LoadFile(previousPersistMetadata, &addresses, persistFile)
		for _, address := range addresses {
			data.Nodes = append(data.Nodes, nodePersist{
				NetAddress: address,
				Inbound:    true,
			})
		}
	}

	if err != nil {
		return err
	}

	// add saved nodes
	for _, node := range data.Nodes {
		if !node.Inbound {
			// if outbound node, try to connect to it
			err := g.Connect(node.NetAddress)
			if err == nil {
				continue
			}

			g.log.Printf("WARN: error connecting outbound '%v' from persist: %v", node.NetAddress, err)
		}

		err := g.addNode(node.NetAddress, node.Inbound)
		if err != nil {
			g.log.Printf("WARN: error loading node '%v' from persist: %v", node.NetAddress, err)
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
