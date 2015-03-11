package gateway

import (
	"io/ioutil"
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

func (g *Gateway) load(filename string) (err error) {
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
