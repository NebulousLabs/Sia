package host

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"
)

// compat04Metadata is the header used by 0.4.x versions of the host.
var compat04Metadata = persist.Metadata{
	Header:  "Sia Host",
	Version: "0.4",
}

// compat04Obligation contains the fields that were used by the 0.4.x to
// represent contract obligations.
type compat04Obligation struct {
	ID           types.FileContractID
	FileContract types.FileContract
	Path         string
}

// compat04Host contains the fields that were saved to disk by 0.4.x hosts.
type compat04Host struct {
	SpaceRemaining int64
	FileCounter    int
	Profit         types.Currency
	HostSettings   modules.HostSettings
	Obligations    []compat04Obligation
	SecretKey      crypto.SecretKey
	PublicKey      types.SiaPublicKey
}

// loadCompat04Obligations loads all of the file contract obligations found by
func (h *Host) loadCompat04Obligations(c04os []compat04Obligation) []*contractObligation {
	cos := make([]*contractObligation, 0, len(c04os))
	for _, c04o := range c04os {
		// Create an upgraded contract obligation out of the compatibility
		// obligation and add it to the set of upgraded obligations.
		co := &contractObligation{
			ID: c04o.ID,
			OriginTransaction: types.Transaction{
				FileContracts: []types.FileContract{
					c04o.FileContract,
				},
			},

			Path: c04o.Path,
		}

		// Update the statistics of the obligation, but do not add any action
		// items for the obligation. Action items will be added after the
		// consensus set has been scanned. Scanning the consensus set will
		// reveal which obligations have transactions on the blockchain, all
		// other obligations will be discarded.
		h.obligationsByID[co.ID] = co
		h.spaceRemaining -= int64(co.fileSize())
		h.anticipatedRevenue = h.anticipatedRevenue.Add(co.value())
		cos = append(cos, co)
	}
	return cos
}

// compatibilityLoad tries to load the file as a compatible version.
func (h *Host) compatibilityLoad() error {
	// Try loading the file as a 0.4 file.
	c04h := new(compat04Host)
	err := persist.LoadFile(compat04Metadata, c04h, filepath.Join(h.persistDir, "settings.json"))
	if err != nil {
		// 0.4.x is the only backwards compatibility provided. File could not
		// be loaded.
		return err
	}

	// Copy over host identity.
	h.publicKey = c04h.PublicKey
	h.secretKey = c04h.SecretKey

	// Copy file management, including providing compatibility for old
	// obligations.
	h.fileCounter = int64(c04h.FileCounter)
	h.spaceRemaining = c04h.HostSettings.TotalStorage
	upgradedObligations := h.loadCompat04Obligations(c04h.Obligations)

	// Copy over statistics.
	h.revenue = c04h.Profit

	// Copy over utilities.
	h.settings = c04h.HostSettings
	// AcceptingContracts should be true by default
	h.settings.AcceptingContracts = true

	// Subscribe to the consensus set.
	if build.DEBUG && h.recentChange != (modules.ConsensusChangeID{}) {
		panic("compatibility loading is not starting from blank consensus?")
	}
	// initConsensusSubscription will scan through the consensus set and set
	// 'OriginConfirmed' and 'RevisionConfirmed' when the accociated file
	// contract and file contract revisions are found.
	err = h.initConsensusSubscription()
	if err != nil {
		return err
	}

	// Remove all obligations that have not had their origin transactions
	// confirmed on the blockchain, and add action items for the rest.
	for _, uo := range upgradedObligations {
		if !uo.OriginConfirmed {
			// Because there is no transaction on the blockchain, and because
			// the old host did not keep the transaction, it's highly unlikely
			// that a transaction will appear - the obligation should be
			// removed.
			h.removeObligation(uo, obligationUnconfirmed)
			continue
		}
		// Check that the file Merkle root matches the Merkle root found in the
		// blockchain.
		file, err := os.Open(uo.Path)
		if err != nil {
			h.log.Println("Compatibility contract file could not be opened.")
			h.removeObligation(uo, obligationFailed)
			continue
		}
		merkleRoot, err := crypto.ReaderMerkleRoot(file)
		file.Close() // Close the file after use, to prevent buildup if the loop iterates many times.
		if err != nil {
			h.log.Println("Compatibility contract file could not be checksummed")
			h.removeObligation(uo, obligationFailed)
			continue
		}
		if merkleRoot != uo.merkleRoot() {
			h.log.Println("Compatibility contract file has the wrong merkle root")
			h.removeObligation(uo, obligationFailed)
			continue
		}
	}
	return nil
}
