Sia 1.0.3
=========

[![Build Status](https://travis-ci.org/NebulousLabs/Sia.svg?branch=master)](https://travis-ci.org/NebulousLabs/Sia)
[![GoDoc](https://godoc.org/github.com/NebulousLabs/Sia?status.svg)](https://godoc.org/github.com/NebulousLabs/Sia)
[![Go Report Card](https://goreportcard.com/badge/github.com/NebulousLabs/Sia)](https://goreportcard.com/report/github.com/NebulousLabs/Sia)

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

Sia is ready for use with small sums of money and non-critical files, but until
the network has a more proven track record, we advise against using it as a
sole means of storing important data.

This release comes with 2 binaries, siad and siac. siad is a background
service, or "daemon," that runs the Sia protocol, and siac is a client that is
used to interact with siad. Siad exposes an HTTP API on 'localhost:9980' which
can be used to interact with the daemon. There is a front-end program called
Sia-UI which can be used to interact with the daemon in a more user-friendly
way. Documentation on the API can be found in doc/API.md.

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
on the system. Then simply use `go get`:

```
go get -u github.com/NebulousLabs/Sia/...
```

This will download the Sia repo to your `$GOPATH/src` folder, and install the
`siad` and `siac` binaries in your `$GOPATH/bin` folder.

To stay up-to-date, run the previous `go get` command again. Alternatively, you
can use the Makefile provided in this repo. Run `git pull origin master` to
pull the latest changes, and `make release-std` to build the new binaries. You
can also run `make test` and `make test-long` to run the short and full test
suites, respectively. Finally, `make cover` will generate code coverage reports
for each package; they are stored in the `cover` folder and can be viewed in
your browser.

Troubleshooting
---------------

- I can't add storage folders on Windows.

  Currently, admin privileges are required to add storage folders on Windows.
  Close siad and run it from an admin command prompt. Future versions will not
  require admin privileges.

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
  blocks, it means your block did not make it into the blockchain.

If your issue is not addressed above, you can get in touch with us personally:

  slack: http://slackin.siacoin.com (ping @taek, @nemo, or @jordan)

  email:
  
  david@nebulouslabs.com
  
  luke@nebulouslabs.com
  
  jordan@nebulouslabs.com

Version Information
-------------------

- v1.0.0 represents a landmark in the development of Sia. In accordance with
  semver, API compatibility will not be broken until v2.0.0, which is not on
  the current development roadmap. In other words, developers should feel
  confident leveraging the siad API in their own applications. When new
  functionality is added, it will be added in a backwards-compatible manner.
  New routes may be added, and new parameters may be added to existing routes
  or responses, but none of the routes present in v1.0.0 will be removed, nor
  will any of their parameters or response fields be removed.

- siad now supports API authentication, enabled by the `--authenticate-api`
  flag.

Please tell us about any problems you run into, and any features you want! The
advantage of being a beta user is that your feedback will have a large impact
on what we do in the next few months. Thank you!

Version History
---------------

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

For older release notes, consult the git history of this file.
