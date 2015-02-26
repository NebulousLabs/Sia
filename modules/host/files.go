package host

import (
	"io/ioutil"

	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/modules"
)

type savedHost struct {
	SpaceRemaining int64
	FileCounter    int
	Obligations    []contractObligation
	HostSettings   modules.HostSettings
}

func (h *Host) save(filename string) (err error) {
	sHost := savedHost{
		SpaceRemaining: h.spaceRemaining,
		FileCounter:    h.fileCounter,
		Obligations:    make([]contractObligation, 0, len(h.obligationsByID)),
		HostSettings:   h.HostSettings,
	}
	for _, obligation := range h.obligationsByID {
		sHost.Obligations = append(sHost.Obligations, obligation)
	}

	err = ioutil.WriteFile(filename, encoding.Marshal(sHost), 0666)
	if err != nil {
		return
	}

	return
}

func (h *Host) load(filename string) (err error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return
	}
	var sHost savedHost
	err = encoding.Unmarshal(contents, &sHost)
	if err != nil {
		return
	}

	h.spaceRemaining = sHost.SpaceRemaining
	h.fileCounter = sHost.FileCounter
	h.HostSettings = sHost.HostSettings
	// recreate maps
	for _, obligation := range sHost.Obligations {
		height := obligation.fileContract.Start + StorageProofReorgDepth
		h.obligationsByHeight[height] = append(h.obligationsByHeight[height], obligation)
		h.obligationsByID[obligation.id] = obligation
	}

	return
}
