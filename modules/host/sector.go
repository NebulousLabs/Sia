package host

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"path/filepath"

	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"

	"github.com/NebulousLabs/bolt"
)

// TODO: Write a sector consistency check - every sector in the host database
// should be represented by a sector on disk, and vice-versa. This is closer to
// a testing check, because the host is tolerant of disk corruption - it is
// okay for there to be information in the sector usage struct that cannot be
// retrieved from the disk. The consistency check should return information on
// how much corruption there is an what shape it takes. If there are files
// found on disk that are not represented in the usage struct, those files
// should be reported as well. The consistency check should be acoompanied by a
// 'purge' mode (perhaps multiple modes) which will delete any files in the
// storage folders which are not represented in the sector usage database.
//
// A simliar check should exist for verifying that the host has the correct
// folder structure. All of the standard files, plus all of the storage
// folders, nothing more. This check belongs in storagefolders.go
//
// A final check, the obligations check, should verify that every sector in the
// sector usage database is represented correctly by the storage obligations,
// and that every sector in the storage obligations is represented by the
// sector usage database.
//
// Disk inconsistencies should be handled by returning errors when trying to
// read from the filesystem, which means the problem manifests at the lowest
// level, the sector level. Because data is missing, there is no 'repair'
// operation that can be supported. The sector usage database should match the
// storage obligation database, and should be patched if there's a mismatch.
// The storage obligation database gets preference. Any missing sectors will be
// treated as if they were filesystem problems.
//
// The consistency check should be wary of 'SizeRemaining' when it is trying to
// do cleanup - if sector removals fail, SizeRemaining should not update as
// though the sectors are gone (but should also be correct such that it's the
// size of the real sectors + the size of the unremovable files - calculated,
// not relative)

// TODO: Write an RPC that lets the host share which sectors it has lost.

var (
	// errDiskTrouble is returned when the host is supposed to have enough
	// storage to hold a new sector but failures that are likely related to the
	// disk have prevented the host from successfully adding the sector.
	errDiskTrouble = errors.New("host unable to add sector despite having the storage capacity to do so")

	// errInsufficientStorageForSector is returned if the host tries to add a
	// sector when there is not enough storage remaining on the host to accept
	// the sector.
	//
	// Ideally, the host will adjust pricing as the host starts to fill up, so
	// this error should be pretty rare. Demand should drive the price up
	// faster than the Host runs out of space, such that the host is always
	// hovering around 95% capacity and rarely over 98% or under 90% capacity.
	errInsufficientStorageForSector = errors.New("not enough storage remaining to accept sector")

	// errMaxVirtualSectors is returned when a sector cannot be added because
	// the maximum number of virtual sectors for that sector id already exist.
	errMaxVirtualSectors = errors.New("sector collides with a physical sector that already has the maximum allowed number of virtual sectors")

	// errSectorNotFound is returned when a lookup for a sector fails.
	errSectorNotFound = errors.New("could not find the desired sector")
)

// sectorUsage indicates how a sector is being used. Each block height
// represents a point at which a file contract using the sector expires. File
// contracts that use the sector multiple times will have their block height
// appear multiple times. This data allows the host to figure out what types of
// discounts can be applied to data that is reusing sectors. This is primarily
// useful for file contract renewals, and really shouldn't be used otherwise.
//
// The StorageFolder field indicates which storage folder is housing the
// sector.
type sectorUsage struct {
	Corrupted     bool // If the corrupted flag is set, it means the sector is permanently unreachable.
	Expiry        []types.BlockHeight
	StorageFolder []byte
}

// sectorID returns the id that should be used when referring to a sector.
// There are lots of sectors, and to minimize their footprint a reduced size
// hash is used. Hashes are typically 256bits to provide collision resistance
// against an attacker that is able to peform an obscene number of trials per
// second on each of an obscene number of machines. Potential collisions for
// sectors are limited because hosts have secret data that the attacker does
// not know which is used to salt the transformation of a sector hash to a
// sectorID. As a result, an attacker is limited in the number of times they
// can try to cause a collision - one random shot every time they upload a
// sector, and the attacker has limited ability to learn of the success of the
// attempt. Uploads are very slow, even on fast machines there will be less
// than 1000 per second. It is therefore safe to reduce the security from
// 256bits to 96bits, which has a collision resistance of 2^48. A reasonable
// upper bound for the number of sectors on a host is 2^32, corresponding with
// 16PB of data.
//
// 12 bytes can be represented as a filepath using 16 base64 characters. This
// keeps the filesize small and therefore limits the amount of load placed on
// the filesystem when trying to manage hundreds of thousands or even tens of
// millions of sectors in a single folder.
func (h *Host) sectorID(sectorRootBytes []byte) []byte {
	saltedRoot := crypto.HashAll(sectorRootBytes, h.sectorSalt)
	id := make([]byte, base64.RawURLEncoding.EncodedLen(12))
	base64.RawURLEncoding.Encode(id, saltedRoot[:12])
	return id
}

// addSector will add a data sector to the host, correctly selecting the
// storage folder in which the sector belongs.
func (h *Host) addSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight, sectorData []byte) error {
	// Sanity check - sector should have sectorSize bytes.
	if uint64(len(sectorData)) != sectorSize {
		h.log.Critical("incorrectly sized sector passed to addSector in the host")
		return errors.New("incorrectly sized sector passed to addSector in the host")
	}

	// Check that there is enough room for the sector in at least one storage
	// folder - check will also guarantee that there is at least one storage folder.
	enoughRoom := false
	for _, sf := range h.storageFolders {
		if sf.SizeRemaining >= sectorSize {
			enoughRoom = true
		}
	}
	if !enoughRoom {
		return errInsufficientStorageForSector
	}

	// Determine which storage folder is going to receive the new sector.
	err := h.db.Update(func(tx *bolt.Tx) error {
		// Check whether the sector is a virtual sector.
		sectorKey := h.sectorID(sectorRoot[:])
		bsu := tx.Bucket(bucketSectorUsage)
		usageBytes := bsu.Get(sectorKey)
		var usage sectorUsage
		if usageBytes != nil {
			// usageBytes != nil indicates that this sector is already a in the
			// database, meaning it's a virtual sector. Add the expiration
			// height to the list of expirations, and then return.
			err := json.Unmarshal(usageBytes, &usage)
			if err != nil {
				return err
			}
			// If the sector already has the maximum number of virtual sectors,
			// return an error. The host handles virtual sectors differently
			// from physical sectors and therefore needs to limit the number of
			// times that the same data can be uploaded to the host. For
			// renters that are properly using encryption and are using
			// sane/reasonable file contract renewal practices, this limit will
			// never be reached (sane behavior will cause 3-5 at an absolute
			// maximum, but the limit is substantially higher).
			if len(usage.Expiry) >= maximumVirtualSectors {
				return errMaxVirtualSectors
			}
			usage.Expiry = append(usage.Expiry, expiryHeight)
			usageBytes, err = json.Marshal(usage)
			if err != nil {
				return err
			}
			return bsu.Put(h.sectorID(sectorRoot[:]), usageBytes)
		}

		// Try adding the sector to disk. In the event of a failure, the host
		// will try the next storage folder until there is either a success or
		// until all options have been exhausted.
		potentialFolders := h.storageFolders
		emptiestFolder, emptiestIndex := emptiestStorageFolder(potentialFolders)
		for emptiestFolder != nil {
			sectorPath := filepath.Join(h.persistDir, emptiestFolder.uidString(), string(sectorKey))
			err := h.dependencies.writeFile(sectorPath, sectorData, 0700)
			if err != nil {
				// Indicate to the user that the storage folder is having write
				// trouble.
				emptiestFolder.FailedWrites++

				// Remove the attempted write - an an incomplete write can
				// leave a partial file on disk. Error is not checked, we
				// already know the disk is having trouble.
				_ = h.dependencies.removeFile(sectorPath)

				// Remove the failed folder from the list of folders that can
				// be tried.
				potentialFolders = append(potentialFolders[0:emptiestIndex], potentialFolders[emptiestIndex+1:]...)

				// Try the next folder.
				emptiestFolder, emptiestIndex = emptiestStorageFolder(potentialFolders)
				continue
			}
			emptiestFolder.SuccessfulWrites++

			// File write succeeded - add the sector to the sector usage
			// database and return.
			usage := sectorUsage{
				Expiry:        []types.BlockHeight{expiryHeight},
				StorageFolder: emptiestFolder.UID,
			}
			emptiestFolder.SizeRemaining -= sectorSize
			usageBytes, err = json.Marshal(usage)
			if err != nil {
				return err
			}
			return bsu.Put(sectorKey, usageBytes)
		}

		// There is at least one disk that has room, but the write operation
		// has failed.
		return errDiskTrouble
	})
	if err != nil {
		return err
	}
	return h.save()
}

// readSector will pull a sector from disk into memory.
func (h *Host) readSector(sectorRoot crypto.Hash) (sectorBytes []byte, err error) {
	err = h.db.View(func(tx *bolt.Tx) error {
		bsu := tx.Bucket(bucketSectorUsage)
		sectorKey := h.sectorID(sectorRoot[:])
		sectorUsageBytes := bsu.Get(sectorKey)
		if sectorUsageBytes == nil {
			return errSectorNotFound
		}
		var su sectorUsage
		err = json.Unmarshal(sectorUsageBytes, &su)
		if sectorUsageBytes == nil {
			return err
		}

		sectorPath := filepath.Join(h.persistDir, hex.EncodeToString(su.StorageFolder), string(sectorKey))
		sectorBytes, err = ioutil.ReadFile(sectorPath)
		if err != nil {
			// TODO: Mark the read failure in the sector?
			return err
		}
		return nil
	})
	return
}

// removeSector will remove a sector from the host at the given expiry height.
// If the provided sector does not have an expiration at the given height, an
// error will be thrown.
func (h *Host) removeSector(sectorRoot crypto.Hash, expiryHeight types.BlockHeight) error {
	// Open the database so that the sector usage information can be updated.
	return h.db.Update(func(tx *bolt.Tx) error {
		// Grab the existing sector usage information from the database.
		bsu := tx.Bucket(bucketSectorUsage)
		sectorKey := h.sectorID(sectorRoot[:])
		sectorUsageBytes := bsu.Get(sectorKey)
		if sectorUsageBytes == nil {
			return errSectorNotFound
		}
		var usage sectorUsage
		err := json.Unmarshal(sectorUsageBytes, &usage)
		if err != nil {
			return err
		}
		if len(usage.Expiry) == 0 {
			h.log.Critical("sector recorded in database, but has no expirations")
			return errSectorNotFound
		}
		if len(usage.Expiry) == 1 && usage.Expiry[0] != expiryHeight {
			return errSectorNotFound
		}

		// If there are multiple entries in the usage expiry, it means that the
		// physcial data is in use by other sectors ('virtual sectors'). This
		// sector can be removed from the usage expiry, but the physical data
		// needs to remain.
		if len(usage.Expiry) > 1 {
			// Find any single entry in the usage that's at the expiry height
			// and remove it.
			var i int
			found := false
			for i = 0; i < len(usage.Expiry); i++ {
				if usage.Expiry[i] == expiryHeight {
					found = true
					break
				}
			}
			if !found {
				return errSectorNotFound
			}
			usage.Expiry = append(usage.Expiry[0:i], usage.Expiry[i+1:]...)

			// Update the database with the new usage expiry.
			sectorUsageBytes, err = json.Marshal(usage)
			if err != nil {
				return err
			}
			return bsu.Put(sectorKey, sectorUsageBytes)
		}

		// Get the storage folder that contains the phsyical sector.
		var folder *storageFolder
		for _, sf := range h.storageFolders {
			if bytes.Equal(sf.UID, usage.StorageFolder) {
				folder = sf
			}
		}

		// Remove the sector from the physical disk.
		sectorPath := filepath.Join(h.persistDir, hex.EncodeToString(usage.StorageFolder), string(sectorKey))
		err = h.dependencies.removeFile(sectorPath)
		if err != nil {
			// Indicate that the storage folder is having write troubles.
			folder.FailedWrites++
			return err
		}

		// Find the storage folder that contains the sector.
		folder.SizeRemaining += sectorSize
		folder.SuccessfulWrites++
		err = h.save()
		if err != nil {
			return err
		}

		// Delete the sector from the bucket - there are no more instances of
		// this sector in the host.
		return bsu.Delete(h.sectorID(sectorRoot[:]))
	})
}
