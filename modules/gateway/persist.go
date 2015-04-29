package gateway

import (
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

func (g *Gateway) save() error {
	var nodes []modules.NetAddress
	for node := range g.nodes {
		nodes = append(nodes, node)
	}
	return encoding.WriteFile(filepath.Join(g.saveDir, "nodes.dat"), nodes)
}

func (g *Gateway) load() error {
	var nodes []modules.NetAddress
	err := encoding.ReadFile(filepath.Join(g.saveDir, "nodes.dat"), &nodes)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		g.addNode(node)
	}
	return nil
}

// create logger
// TODO: when is the logFile closed? Does it need to be closed?
func makeLogger(saveDir string) (*log.Logger, error) {
	logFile, err := os.OpenFile(filepath.Join(saveDir, "gateway.log"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		return nil, err
	}
	return log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile), nil
}
