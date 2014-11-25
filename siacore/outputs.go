package siacore

import (
	"errors"
)

// ScanOutputs takes a map of coin addresses as input and returns every output
// in the set of unspent outputs that matches the list of addresses.
func (s *State) ScanOutputs(addresses map[CoinAddress]struct{}) (outputIDs []OutputID) {
	for id, output := range s.unspentOutputs {
		if _, exists := addresses[output.SpendHash]; exists {
			outputIDs = append(outputIDs, id)
		}
	}

	return
}

// State.Output returns the Output associated with the id provided for input,
// but only if the output is a part of the utxo set.
func (s *State) Output(id OutputID) (output Output, err error) {
	output, exists := s.unspentOutputs[id]
	if exists {
		return
	}

	err = errors.New("output not in utxo set")
	return
}
