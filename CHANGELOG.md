Version History
---------------

May 2018:

v1.3.3 (patch release)
- Add Streaming API endpoints
- Faster contract formation
- Improved wallet scaling

March 2018:

v1.3.2 (patch release)
- Improve renter throughput and stability
- Reduce host I/O when idle
- Add /tpool/confirmed endpoint

December 2017:

v1.3.1 (patch release)
- Add new efficient, reliable contract format
- Faster and smoother file repairs
- Fix difficulty adjustment hardfork

July 2017:

v1.3.0 (minor release)
- Add remote file repair
- Add wallet 'lookahead'
- Introduce difficulty hardfork

May 2017:

v1.2.2 (patch release)
- Faster + smaller wallet database
- Gracefully handle missing storage folders
- >2500 lines of new testing + bug fixes

April 2017:

v1.2.1 (patch release)
- Faster host upgrading
- Fix wallet bugs
- Add siac command to cancel allowance

v1.2.0 (minor release)
- Host overhaul
- Wallet overhaul
- Tons of bug fixes and efficiency improvements

March 2017:

v1.1.2 (patch release)
- Add async download endpoint
- Fix host storage proof bug

February 2017:

v1.1.1 (patch release)
- Renter now performs much better at scale
- Myriad HostDB improvements
- Add siac command to support storage leaderboard

January 2017:

v1.1.0 (minor release)
- Greatly improved upload/download speeds
- Wallet now regularly "defragments"
- Better contract metrics

December 2016:

v1.0.4 (LTS release)

October 2016:

v1.0.3 (patch release)
- Greatly improved renter stability
- Smarter HostDB
- Numerous minor bug fixes

July 2016:

v1.0.1 (patch release)
- Restricted API address to localhost
- Fixed renter/host desynchronization
- Fixed host silently refusing new contracts

June 2016:

v1.0.0 (major release)
- Finalized API routes
- Add optional API authentication
- Improve automatic contract management

May 2016:

v0.6.0 (minor release)
- Switched to long-form renter contracts
- Added support for multiple hosting folders
- Hosts are now identified by their public key

January 2016:

v0.5.2 (patch release)
- Faster initial blockchain download
- Introduced headers-only broadcasting

v0.5.1 (patch release)
- Fixed bug severely impacting performance
- Restored (but deprecated) some siac commands
- Added modules flag, allowing modules to be disabled

v0.5.0 (minor release)
- Major API changes to most modules
- Automatic contract renewal
- Data on inactive hosts is reuploaded
- Support for folder structure
- Smarter host

October 2015:

v0.4.8 (patch release)
- Restored compatibility with v0.4.6

v0.4.7 (patch release)
- Dropped support for v0.3.3.x

v0.4.6 (patch release)
- Removed over-aggressive consistency check

v0.4.5 (patch release)
- Fixed last prominent bug in block database
- Closed some dangling resource handles

v0.4.4 (patch release)
- Uploading is much more reliable
- Price estimations are more accurate
- Bumped filesize limit to 20 GB

v0.4.3 (patch release)
- Block database is now faster and more stable
- Wallet no longer freezes when unlocked during IBD
- Optimized block encoding/decoding

September 2015:

v0.4.2 (patch release)
- HostDB is now smarter
- Tweaked renter contract creation

v0.4.1 (patch release)
- Added support for loading v0.3.3.x wallets
- Better pruning of dead nodes
- Improve database consistency

August 2015:

v0.4.0: Second stable currency release.
- Wallets are encrypted and generated from seed phrases
- Files are erasure-coded and transferred in parallel
- The blockchain is now fully on-disk
- Added UPnP support

June 2015:

v0.3.3.3 (patch release)
- Host announcements can be "forced"
- Wallets can be merged
- Unresponsive addresses are pruned from the node list

v0.3.3.2 (patch release)
- Siafunds can be loaded and sent
- Added block explorer
- Patched two critical security vulnerabilities

v0.3.3.1 (hotfix)
- Mining API sends headers instead of entire blocks
- Slashed default hosting price

v0.3.3: First stable currency release.
- Set release target
- Added progress bars to uploads
- Rigorous testing of consensus code

May 2015:

v0.3.2: Fourth open beta release.
- Switched encryption from block cipher to stream cipher
- Updates are now signed
- Added API calls to support external miners

v0.3.1: Third open beta release.
- Blocks are now stored on-disk in a database
- Files can be shared via .sia files or ASCII-encoded data
- RPCs are now multiplexed over one physical connection

March 2015:

v0.3.0: Second open beta release.

Jan 2015:

v0.2.0: First open beta release.

Dec 2014:

v0.1.0: Closed beta release.
