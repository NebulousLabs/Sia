package gateway

import (
	"errors"
	"io/ioutil"
	"math/rand"
	"sync"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/network"
)

func (g *Gateway) addPeer(peer network.Address) error {
	if _, exists := g.peers[peer]; exists {
		return errors.New("peer already added")
	}
	g.peers[peer] = 0
	return nil
}

func (g *Gateway) removePeer(peer network.Address) error {
	if _, exists := g.peers[peer]; !exists {
		return errors.New("no record of that peer")
	}
	delete(g.peers, peer)
	return nil
}

func (g *Gateway) randomPeer() (network.Address, error) {
	r := rand.Intn(len(g.peers))
	for peer := range g.peers {
		if r == 0 {
			return peer, nil
		}
		r--
	}
	return "", ErrNoPeers
}

// threadedBroadcast calls an RPC on all of the peers in the Gateway's peer
// list. The calls are run in parallel.
func (g *Gateway) threadedBroadcast(name string, arg, resp interface{}) {
	var badpeers []network.Address
	var wg sync.WaitGroup
	wg.Add(len(g.peers))

	g.mu.RLock()
	for peer := range g.peers {
		// contact each peer in a separate thread
		go func(peer network.Address) {
			err := peer.RPC(name, arg, resp)
			// TODO: some errors will be our fault. Need to distinguish them.
			if err != nil {
				badpeers = append(badpeers, peer)
			}
			wg.Done()
		}(peer)
	}
	g.mu.RUnlock()
	wg.Wait()

	// process the bad peers
	g.mu.Lock()
	for _, peer := range badpeers {
		g.peers[peer]++ // increment strikes
		if g.peers[peer] > maxStrikes {
			g.removePeer(peer)
		}
	}
	g.mu.Unlock()
}

func (g *Gateway) save(filename string) error {
	peers := g.Info().Peers
	return ioutil.WriteFile(filename, encoding.Marshal(peers), 0666)
}

func (g *Gateway) load(filename string) (err error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	var peers []network.Address
	err = encoding.Unmarshal(contents, &peers)
	if err != nil {
		return
	}
	for _, peer := range peers {
		g.addPeer(peer)
	}
	return
}
