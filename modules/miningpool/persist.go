package pool

import (
	"os"
	"path/filepath"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/modules"
	"github.com/NebulousLabs/Sia/persist"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// persistence is the data that is kept when the pool is restarted.
type persistence struct {
	// Consensus Tracking.
	BlockHeight  types.BlockHeight         `json:"blockheight"`
	RecentChange modules.ConsensusChangeID `json:"recentchange"`

	// Host Identity.
	Announced      bool                         `json:"announced"`
	AutoAddress    modules.NetAddress           `json:"autoaddress"`
	MiningMetrics  modules.PoolMiningMetrics    `json:"miningmetrics"`
	PublicKey      types.SiaPublicKey           `json:"publickey"`
	RevisionNumber uint64                       `json:"revisionnumber"`
	Settings       modules.PoolInternalSettings `json:"settings"`
	UnlockHash     types.UnlockHash             `json:"unlockhash"`
	Height         types.BlockHeight
	Target         types.Target
	Address        types.UnlockHash
	BlocksFound    []types.BlockID
	UnsolvedBlock  types.Block
}

// persistData returns the data in the Host that will be saved to disk.
func (mp *Pool) persistData() persistence {
	return persistence{
		// Consensus Tracking.
		BlockHeight:  mp.blockHeight,
		RecentChange: mp.recentChange,

		// Host Identity.
		Announced:      mp.announced,
		AutoAddress:    mp.autoAddress,
		MiningMetrics:  mp.miningMetrics,
		PublicKey:      mp.publicKey,
		RevisionNumber: mp.revisionNumber,
		Settings:       mp.settings,
		UnlockHash:     mp.unlockHash,
	}
}

// establishDefaults configures the default settings for the pool, overwriting
// any existing settings.
func (mp *Pool) establishDefaults() error {
	// Configure the settings object.
	mp.settings = modules.PoolInternalSettings{
		AcceptingShares:     false,
		PoolOwnerPercentage: 0.0,
		PoolOwnerWallet:     "",

		PoolNetworkPort: 0,
	}

	return nil
}

// loadPersistObject will take a persist object and copy the data into the
// host.
func (mp *Pool) loadPersistObject(p *persistence) {
	// Copy over consensus tracking.
	mp.blockHeight = p.BlockHeight
	mp.recentChange = p.RecentChange

	// Copy over host identity.
	mp.announced = p.Announced
	mp.autoAddress = p.AutoAddress
	if err := p.AutoAddress.IsValid(); err != nil {
		mp.log.Printf("WARN: AutoAddress '%v' loaded from persist is invalid: %v", p.AutoAddress, err)
		mp.autoAddress = ""
	}
	mp.miningMetrics = p.MiningMetrics
	mp.publicKey = p.PublicKey
	mp.revisionNumber = p.RevisionNumber
	mp.settings = p.Settings
	mp.unlockHash = p.UnlockHash
}

// initDB will check that the database has been initialized and if not, will
// initialize the database.
func (mp *Pool) initDB() (err error) {
	// Open the pool's database and set up the stop function to close it.
	mp.db, err = mp.dependencies.openDatabase(dbMetadata, filepath.Join(mp.persistDir, dbFilename))
	if err != nil {
		return err
	}
	mp.tg.AfterStop(func() {
		err = mp.db.Close()
		if err != nil {
			mp.log.Println("Could not close the database:", err)
		}
	})

	return mp.db.Update(func(tx *bolt.Tx) error {
		// The storage obligation bucket does not exist, which means the
		// database needs to be initialized. Create the database buckets.
		buckets := [][]byte{
			bucketActionItems,
			bucketStorageObligations,
		}
		for _, bucket := range buckets {
			_, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// load loads the Hosts's persistent data from disk.
func (mp *Pool) load() error {
	// Initialize the host database.
	err := mp.initDB()
	if err != nil {
		err = build.ExtendErr("Could not initialize database:", err)
		mp.log.Println(err)
		return err
	}

	// Load the old persistence object from disk. Simple task if the version is
	// the most recent version, but older versions need to be updated to the
	// more recent structures.
	p := new(persistence)
	err = mp.dependencies.loadFile(persistMetadata, p, filepath.Join(mp.persistDir, settingsFile))
	if err == nil {
		// Copy in the persistence.
		mp.loadPersistObject(p)
	} else if os.IsNotExist(err) {
		// There is no pool.json file, set up sane defaults.
		return mp.establishDefaults()
	} else if err != nil {
		return err
	}

	return nil
}

// saveSync stores all of the persist data to disk and then syncs to disk.
func (mp *Pool) saveSync() error {
	return persist.SaveJSON(persistMetadata, mp.persistData(), filepath.Join(mp.persistDir, settingsFile))
}
