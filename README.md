Sia 0.6.0
=========

[![Build Status](https://travis-ci.org/NebulousLabs/Sia.svg?branch=master)](https://travis-ci.org/NebulousLabs/Sia)
[![GoDoc](https://godoc.org/github.com/NebulousLabs/Sia?status.svg)](https://godoc.org/github.com/NebulousLabs/Sia)

Binaries can be found at [our website](http://siacoin.com). Code for the graphical front-end can be found at the [Sia-UI](https://github.com/NebulousLabs/Sia-UI) repo.

Sia is a new decentralized cloud storage platform aimed at giving users control
of their data. Data is split into hundreds of erasure coded pieces and
encrypted locally, and then each piece is uploaded to a separate host. A
blockchain is used to create cryptographic contracts ensuring that hosts will
only get paid if they actually store the data. Out of hundreds of hosts, only a
fraction are required to recover the original file.

Anybody can join the network as a host and get income from the storage they
contribute. This openness allows Sia to build and take advantage of a global
network of small datacenters. Combined with advanced algorithms for storing and
retrieving data, Sia is poised to be a highly competitive cloud storage
platform. More information about the technology can be found on our website and
in the 'doc' folder of the repo.

Sia is currently in beta. The currency was launched on June 7th, 2015, but the
storage platform itself remains in beta. Sia is ready for use with small sums
of money and non-critical files, but until the network has a more proven track
record, we advise against using it as a sole means of storing important data.

This release comes with 2 binaries, siad and siac. siad is a background
service, or "daemon," that runs the Sia protocol, and siac is a client that is
used to interact with siad. Siad exposes an API on 'localhost:9980' which can
be used to interact with the daemon. There is a front-end program called Sia-UI
which can be used to interact with the daemon in a more user-friendly way.
Documentation on the API can be found in doc/API.md.

Usage
-----

siad and siac are run via command prompt. On Windows, you can just double-
click siad.exe if you don't need to specify any command-line arguments.
Otherwise, navigate to the sia folder and click File->Open command prompt.
Then, start the siad service by entering `siad` and pressing Enter. The
command prompt may appear to freeze; this means siad is waiting for requests.
Windows users may see a warning from the Windows Firewall; be sure to check
both boxes ("Private networks" and "Public networks") and click "Allow
access." You can now run `siac` in a separate command prompt to interact with
siad. From here, you can send money, mine blocks, upload and download
files, and advertise yourself as a host.

Building From Source
--------------------

To build from source, [Go 1.6 must be installed](https://golang.org/doc/install)
on the system. Then run the following to build:

```
go get github.com/NebulousLabs/Sia
cd  $GOPATH/src/github.com/NebulousLabs/Sia/
make dependencies
make
```

To start `siad`, the Sia daemon process, run:

```
$GOPATH/bin/siad
```

With `siad` running, you can run `siac`, the Sia command-line interface from a
separate shell:

```
$GOPATH/bin/siac
```

Troubleshooting
---------------

- The client does not appear to join the network.

  There should be at least one peer online for you to connect to, so if you
  cannot connect, you may be experiencing connection issues. Ensure that you
  are connected to the Internet. If you are confident that your connection is
  okay, contact us! Our server may be experiencing problems.

  You can also opt not to connect to join the network by passing the
  "--no-bootstrap" flag to siad.

- I can't connect to more than 8 peers.

  Once Sia has connected to 8 peers, it will stop trying to form new
  connections, but it will still accept incoming connection requests (up to 128
  total peers). However, if you are behind a firewall, you will not be able to
  accept incoming connections. You must configure your firewall to allow Sia
  connections by forwarding your ports. By default, Sia communicates on ports
  9981 and 9982. The specific instructions for forwarding a port vary by
  router. For more information, consult [this guide](http://portfoward.com).

  Sia currently has support for UPnP. While not all routers support UPnP, a
  majority of users should have their ports automatically forwarded by UPnP.

- I mined a block, but I didn't receive any money.

  There is a 144-block confirmation delay before you will receive siacoins from
  mined blocks. If you still have not received the block reward after 144
  blocks, it means your block did not made it into the blockchain.

If your issue is not addressed above, you can get in touch with us personally:

  slack: http://slackin.siacoin.com (ping taek or nemo)

  email:
  
  david@nebulouslabs.com
  
  luke@nebulouslabs.com

Version Information
-------------------

- The renter and host modules have been significantly changed in v0.6.0.
  The renter now forms long-term file contracts instead of making a new
  contract for each file. This requires the introduction of "allowances,"
  which are used to specify how much money the renter should spend on
  contracts. Before you can upload files, you must first specify an
  allowance. The renter will then form contracts according to the allowance,
  which are used to store the uploaded files.

- The host now supports multiple storage folders. Each folder can be configured
  with a maximum size. If you remove a storage folder, its data will be
  redistributed to the other folders. You should always remove the storage
  folder via the API before deleting its contents on your filesystem. The host
  cannot store any data until you specify at least one storage folder.

  Hosts can also delete individual sectors (identified via Merkle root) in
  order to comply with take-down requests.

- v0.6.0 introduces some changes that are incompatible with previous versions.
  For example, host announcements now include a public key, and are signed with
  that key. This allows hosts to reannounce later with a different IP address.
  The renter-host communication protocols have also changed significantly. For
  these reasons, we recommend that you reupload data stored on v0.5.0 to v0.6.0
  hosts. Apologies for the inconvenience.

Please tell us about any problems you run into, and any features you want! The
advantage of being a beta user is that your feedback will have a large impact
on what we do in the next few months. Thank you!

Version History
---------------

April 2016:

v0.6.0 (minor release)
- Switched to Long-form renter contracts
- Added support for multiple hosting folders
- Hosts are now identified by their public key

January 2016:

v0.5.2 (patch)
- Faster initial blockchain download
- Introduced headers-only broadcasting

v0.5.1 (patch)
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

v0.4.8 (patch)
- Restored compatibility with v0.4.6

v0.4.7 (patch)
- Dropped support for v0.3.3.x

v0.4.6 (patch)
- Removed over-agressive consistency check

v0.4.5 (patch)
- Fixed last prominent bug in block database
- Closed some dangling resource handles

v0.4.4 (patch)
- Uploading is much more reliable
- Price estimations are more accurate
- Bumped filesize limit to 20 GB

v0.4.3 (patch)
- Block database is now faster and more stable
- Wallet no longer freezes when unlocked during IBD
- Optimized block encoding/decoding

September 2015:

v0.4.2 (patch)
- HostDB is now smarter
- Tweaked renter contract creation

v0.4.1 (patch)
- Added support for loading v0.3.3.x wallets
- Better pruning of dead nodes
- Improve database consistency

August 2015:

v0.4.0: Second stable currency release.
- Wallets are encrypted and generated from seed phrases
- Files are erasure-coded and transferred in parallel
- The blockchain is now fully on-disk
- Added UPnP support
