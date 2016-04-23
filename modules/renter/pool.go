package renter

import (
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/modules/renter/contractor"
)

// A hostPool is a collection of active host connections, in the form of
// Editors. The renter uses a hostPool to prevent connecting to the same host
// more than once. This is more efficient, and also makes it easier to
// serialize contract revisions.
type hostPool struct {
	hosts          []contractor.Editor
	blacklist      []modules.NetAddress
	hostContractor hostContractor
	hdb            hostDB
}

// Close closes all of the hostPool's open host connections.
func (p *hostPool) Close() error {
	for _, h := range p.hosts {
		h.Close()
	}
	return nil
}

// add adds a contract's host to the hostPool and returns it as an Editor.
func (p *hostPool) add(contract contractor.Contract) (contractor.Editor, error) {
	for _, h := range p.hosts {
		if h.Address() == contract.IP {
			return h, nil
		}
	}
	hu, err := p.hostContractor.Editor(contract)
	if err != nil {
		p.blacklist = append(p.blacklist, contract.IP)
		return nil, err
	}
	p.hosts = append(p.hosts, hu)
	return hu, nil
}

// uniqueHosts will return up to 'n' unique hosts that are not in 'exclude'.
// The pool draws from its set of active connections first, and then negotiates
// new contracts if more hosts are required. Note that this latter case
// requires network I/O, so the caller should always assume that uniqueHosts
// will block.
func (p *hostPool) uniqueHosts(n int, exclude []modules.NetAddress) (hosts []contractor.Editor) {
	if n == 0 {
		return
	}

	// convert slice to map for easier lookups
	excludeSet := make(map[modules.NetAddress]struct{})
	for _, ip := range exclude {
		excludeSet[ip] = struct{}{}
	}

	// First reuse existing connections.
	for _, h := range p.hosts {
		if _, ok := excludeSet[h.Address()]; ok {
			continue
		}
		hosts = append(hosts, h)
		if len(hosts) >= n {
			return hosts
		}
	}

	// Extend the exclude set with the pool's blacklist, and the hosts we're
	// already connected to.
	for _, ip := range p.blacklist {
		excludeSet[ip] = struct{}{}
	}
	for _, h := range p.hosts {
		excludeSet[h.Address()] = struct{}{}
	}

	// Next try to reuse existing contracts.
	for _, contract := range p.hostContractor.Contracts() {
		if _, ok := excludeSet[contract.IP]; ok {
			continue
		}
		hu, err := p.add(contract)
		if err != nil {
			continue
		}
		hosts = append(hosts, hu)
		if len(hosts) >= n {
			break
		}
	}

	return hosts
}

// newHostPool returns an empty hostPool.
func (r *Renter) newHostPool() *hostPool {
	return &hostPool{
		hostContractor: r.hostContractor,
		hdb:            r.hostDB,
	}
}
