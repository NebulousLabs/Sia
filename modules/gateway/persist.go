package gateway

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

func (g *Gateway) save() error {
	var peers []modules.NetAddress
	for peer := range g.peers {
		peers = append(peers, peer)
	}
	return ioutil.WriteFile(filepath.Join(g.saveDir, "peers.dat"), encoding.Marshal(peers), 0666)
}

func (g *Gateway) load() (err error) {
	contents, err := ioutil.ReadFile(filepath.Join(g.saveDir, "peers.dat"))
	if err != nil {
		return
	}
	var peers []modules.NetAddress
	err = encoding.Unmarshal(contents, &peers)
	if err != nil {
		return
	}
	for _, peer := range peers {
		g.peers[peer] = 0 // TODO: support saving/loading strikes
	}
	return
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
