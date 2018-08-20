package gateway

import (
	"path/filepath"
	"time"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

const (
	// logFile is the name of the log file.
	logFile = modules.GatewayDir + ".log"

	// nodesFile is the name of the file that contains all seen nodes.
	nodesFile = "nodes.json"

	// blacklistFile is the name of the file that contains all blacklisted nodes.
	blacklistFile = "blacklist.json"
)

// persistMetadata contains the header and version strings that identify the
// gateway persist file.
var persistMetadata = persist.Metadata{
	Header:  "Sia Node List",
	Version: "1.3.0",
}

// persistMetadataBlacklist contains the header and version strings that
// identify the gateway persist file.
var persistMetadataBlacklist = persist.Metadata{
	Header:  "Sia Node Blacklist",
	Version: "1.3.2",
}

// persistData returns the data in the Gateway that will be saved to disk.
func (g *Gateway) persistData() (nodes []*node) {
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	return
}

// persistDataBlacklist returns the data of the blacklist that will be saved to
// disk.
func (g *Gateway) persistDataBlacklist() (ips []string) {
	for ip := range g.blacklist {
		ips = append(ips, ip)
	}
	return
}

// load loads the Gateway's persistent data from disk.
func (g *Gateway) load() error {
	// load nodes
	var nodes []*node
	err := persist.LoadJSON(persistMetadata, &nodes, filepath.Join(g.persistDir, nodesFile))
	if err != nil {
		// COMPATv1.3.0
		err = g.loadv033persist()
		if err != nil {
			return err
		}
	}
	for i := range nodes {
		g.nodes[nodes[i].NetAddress] = nodes[i]
	}

	// load blacklist
	var ips []string
	err = persist.LoadJSON(persistMetadataBlacklist, &ips, filepath.Join(g.persistDir, blacklistFile))
	if err != nil {
		return err
	}
	for _, ip := range ips {
		g.blacklist[ip] = struct{}{}
	}
	return nil
}

// saveBlacklist stores the Gateway's blacklisted ips on disk and then syncs to
// disk to minimize the possibility of data loss.
func (g *Gateway) saveBlacklist() error {
	return persist.SaveJSON(persistMetadataBlacklist, g.persistDataBlacklist(), filepath.Join(g.persistDir, blacklistFile))
}

// saveSync stores the Gateway's persistent data on disk, and then syncs to
// disk to minimize the possibility of data loss.
func (g *Gateway) saveSync() error {
	return persist.SaveJSON(persistMetadata, g.persistData(), filepath.Join(g.persistDir, nodesFile))
}

// threadedSaveLoop periodically saves the gateway.
func (g *Gateway) threadedSaveLoop() {
	for {
		select {
		case <-g.threads.StopChan():
			return
		case <-time.After(saveFrequency):
		}

		func() {
			err := g.threads.Add()
			if err != nil {
				return
			}
			defer g.threads.Done()

			g.mu.Lock()
			err = g.saveSync()
			g.mu.Unlock()
			if err != nil {
				g.log.Println("ERROR: Unable to save gateway persist:", err)
			}
		}()
	}
}

// loadv033persist loads the v0.3.3 Gateway's persistent data from disk.
func (g *Gateway) loadv033persist() error {
	var nodes []modules.NetAddress
	err := persist.LoadJSON(persist.Metadata{
		Header:  "Sia Node List",
		Version: "0.3.3",
	}, &nodes, filepath.Join(g.persistDir, nodesFile))
	if err != nil {
		return err
	}
	for _, addr := range nodes {
		err := g.addNode(addr)
		if err != nil {
			g.log.Printf("WARN: error loading node '%v' from persist: %v", addr, err)
		}
	}
	return nil
}
