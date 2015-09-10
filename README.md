Sia 0.4.2
===========

[![Build Status](https://travis-ci.org/NebulousLabs/Sia.svg?branch=master)](https://travis-ci.org/NebulousLabs/Sia)

Binaries can be found at [our website](http://siacoin.com).

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

Troubleshooting
---------------

- The client does not appear to join the network.

  There should be at least one peer online for you to connect to, so if you
  cannot connect, you may be experiencing connection issues. Ensure that you
  are connected to the Internet. If you are confident that your connection is
  okay, contact us! Our server may be experiencing problems.

  You can also opt not to connect to join the network by passing the "-n" flag
  to siad.

- I can't connect to more than 8 peers.

  Once Sia has connected to 8 peers, it will stop trying to form new
  connections, but it will still accept incoming connection requests (up to 128
  total peers). However, if you are behind a firewall, you will not be able to
  accept incoming connections. You must configure your firewall to allow Sia
  connections by forwarding your ports. By default, Sia communicates on ports
  9981 and 9982. The specific instructions for forwarding a port vary by
  router. For more information, consult [this guide](http://portfoward.com).

  In future versions, we will add support for UPnP, which may allow you to
  skip this step if your router supports it.

- I'm 100% sure my ports are open, but Sia won't let me announce as a host.

  siad tries to verify your connectivity by pinging your external IPv4 address.
  This method is sufficient for most people, but for unusual setups it may
  report false negatives. To override this check, you can "force" the
  announcement by running `siac host announce [ip:port]`. You can determine
  your external IP by running `siac gateway` or using a 3rd-party service.

- I mined a block, but I didn't receive any money.

  There is a 144-block confirmation delay before you will receive siacoins from
  mined blocks. If you still have not received the block reward after 144
  blocks, it means your block did not made it into the blockchain.

- siad complains about "locks held too long."

  This is debugging output, and should not occur during normal use. Please
  contact us if this happens.

If your issue is not addressed above, you can get in touch with us personally:

  slack: http://slackin.siacoin.com (ping taek or nemo)

  email:
  
  david@nebulouslabs.com
  
  luke@nebulouslabs.com

Version Information
-------------------

- v0.4.0 introduces wallet seeds, which can be used to regenerate your wallet
  using only a passphrase. To create a wallet, run `siac wallet init`. Be sure
  to write down the passphrase somewhere safe! You will also need it to unlock
  the wallet each time you start siad.

  As a result, compatibility with the old wallet.dat files has been broken. To
  transfer funds from v0.3.3.3 to v0.4.0, you must send them from the v0.3.3.3
  client to a v0.4.0 address. Note that you can run both clients at once (from
  different folders).

- v0.4.0 uses erasure-coding to enable durability, availability, and efficiency
  when uploading and downloading files. However, the new upload and download
  algorithms have not yet been perfected. Specifically, you may observe slower-
  than-expected download speeds. These algorithms will be a top  priority in
  future releases. After all, they are crucial to the Sia platform!

- The format of the block database has changed, which means you will need to
  redownload the entire blockchain. This may take anywhere from 10 minutes to
  6 hours. Check http://explore.siacoin.com to see the current block height.

- Ports are now automatically forwarded using UPnP. If your router supports
  UPnP, you no longer need to manually set up port forwarding. This should
  improve the general health of the network. Note that for now, there is no
  way to disable UPnP, but you can always turn it off in your router settings.

- Much compatibility with the v0.3.3.3 host and renter has been broken. The
  .sia format has changed, the network protocol is different, etc. The bottom
  line is, if you were hosting on v0.3.3.3, you should continue to do so until
  your contracts expire. If you were renting, you should download those files
  with the v0.3.3.3 client and reupload them on v0.4.0. We apologize for the
  inconvenience. Fortunately, storage is cheap and the network is small.

Please tell us about any problems you run into, and any features you want! The
advantage of being a beta user is that your feedback will have a large impact
on what we do in the next few months. Thank you!

Version History
---------------

August 2015:

v0.4.2 (patch)
- Uploading and Hosting Bugfixes
- Consensus Bugfixes

v0.4.1 (patch)
- Minor Bugfixes
- Improvements to Build Process

v0.4.0: Second stable currency release.
- Wallets are encrypted and generated from seed phrases
- Files are erasure-coded and transferred in parallel
- The blockchain is now fully on-disk
- Added UPnP support

June 2015:

v0.3.3.3 (patch)
- Host announcements can be "forced"
- Wallets can be merged
- Unresponsive addresses are pruned from the node list

v0.3.3.2 (patch)
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
