package gateway

import (
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
)

func (g *Gateway) save() error {
	meta := persist.Metadata{
		Header:   "Sia Node List",
		Version:  "0.3.3",
		Filename: filepath.Join(g.saveDir, "nodes.json"),
	}

	var nodes []modules.NetAddress
	for node := range g.nodes {
		nodes = append(nodes, node)
	}
	return persist.Save(meta, nodes)
}

func (g *Gateway) load() error {
	meta := persist.Metadata{
		Header:   "Sia Node List",
		Version:  "0.3.3",
		Filename: filepath.Join(g.saveDir, "nodes.json"),
	}

	var nodes []modules.NetAddress
	err := persist.Load(meta, &nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		g.addNode(node)
	}
	return nil
}

func makeLogger(saveDir string) (*log.Logger, error) {
	// if the log file already exists, append to it
	logFile, err := os.OpenFile(filepath.Join(saveDir, "gateway.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return nil, err
	}
	return log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile), nil
}
