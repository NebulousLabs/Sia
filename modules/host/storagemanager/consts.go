package storagemanager

const (
	// maximumStorageFolders indicates the maximum number of storage folders
	// that the host allows. Some operations, such as creating a new storage
	// folder, take longer if there are more storage folders. Static RAM usage
	// also increases as the number of storage folders increase. For this
	// reason, a limit on the maximum number of storage folders has been set.
	maximumStorageFolders = 100
)

var (
	// maximumStorageFolderSize sets an upper bound on how large storage
	// folders in the host are allowed to be. It makes sure that inputs and
	// constructions are sane. While it's conceivable that someone could create
	// a rig with a single logical storage folder greater than 128 TiB in size
	// in production, it's probably not a great idea, especially when you are
	// allowed to use many storage folders. All told, a single host on today's
	// constants can support up to ~10 PB of storage.
	maximumStorageFolderSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 40 // 1 TiB
		}
		if build.Release == "standard" {
			return 1 << 50 // 1 PiB
		}
		if build.Release == "testing" {
			return 1 << 20 // 1 MiB
		}
		panic("unrecognized release constant in host - maximum storage folder size")
	}()

	// maximumVirtualSectors defines the maximum number of virtual sectors that
	// can be tied to each physical sector.
	maximumVirtualSectors = func() int {
		if build.Release == "dev" {
			// The testing value is at 35 to provide flexibility. The
			// development value is at 5 because hitting the virtual sector
			// limit in a sane development environment is more difficult than
			// hitting the virtual sector limit in a controlled testing
			// environment (dev environment doesn't have access to private
			// methods such as 'addSector'.
			return 5
		}
		if build.Release == "standard" {
			// Each virtual sector adds about 8 bytes of load to the host
			// persistence structures, and additionally adds 8 bytes of load
			// when reading or modifying a sector. Though a few virtual sectors
			// with 10e3 or even 100e3 virtual sectors would not be too
			// detrimental to the host, tens of thousands of physical sectors
			// that each have ten thousand virtual sectors could pose a problem
			// for the host. In most situations, a renter will not need more 2
			// or 3 virtual sectors when manipulating data, so 250 is generous
			// as long as the renter is properly encrypting data. 250 is
			// unlikely to cause the host problems, even if an attacker is
			// creating hundreds of thousands of phsyical sectors (an expensive
			// action!) each with 250 vitrual sectors.
			return 250
		}
		if build.Release == "testing" {
			return 35
		}
		panic("unrecognized release constant in host - maximum virtual sector size")
	}()

	// minimumStorageFolderSize defines the smallest size that a storage folder
	// is allowed to be. The new design of the storage folder structure means
	// that this limit is not as relevant as it was originally, but hosts with
	// little storage capacity are not very useful to the network, and can
	// actually frustrate price planning. 32 GB has been chosen as a minimum
	// for the early days of the network, to allow people to experiment in the
	// beta, but in the future I think something like 256GB would be much more
	// appropraite.
	minimumStorageFolderSize = func() uint64 {
		if build.Release == "dev" {
			return 1 << 25 // 32 MiB
		}
		if build.Release == "standard" {
			return 1 << 35 // 32 GiB
		}
		if build.Release == "testing" {
			return 1 << 15 // 32 KiB
		}
		panic("unrecognized release constant in host - minimum storage folder size")
	}()

	// storageFolderUIDSize determines the number of bytes used to determine
	// the storage folder UID. Production and development environments use 4
	// bytes to minimize the possibility of accidental collisions, and testing
	// environments use 1 byte so that collisions can be forced while using the
	// live code.
	storageFolderUIDSize = func() int {
		if build.Release == "dev" {
			return 2
		}
		if build.Release == "standard" {
			return 4
		}
		if build.Release == "testing" {
			return 1
		}
		panic("unrecognized release constant in host - storageFolderUIDSize")
	}()

	// bucketSectorUsage maps sector IDs to the number of times they are used
	// in file contracts. If all data is correctly encrypted using a unique
	// seed, each sector will be in use exactly one time. The host however
	// cannot control this, and a user may upload unencrypted data or
	// intentionally upload colliding sectors as a means of attack. The host
	// can only delete a sector when it is in use zero times. The number of
	// times a sector is in use is encoded as a big endian uint64.
	bucketSectorUsage = []byte("BucketSectorUsage")
)
