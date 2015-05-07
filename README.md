Sia 0.3.1
=========

[![Build Status](https://travis-ci.org/NebulousLabs/Sia.svg?branch=master)](https://travis-ci.org/NebulousLabs/Sia)

Binaries can be found at [our website](http://siacoin.com).

Sia is a new cryptosystem designed to enable incentivized, decentralized
storage in a Byzantine environment. The consensus protocol has been finished
and is partially explained in Consensus.md. There is a working reference
implementation in the consensus folder. While this implementation is well-
tested, it is not guaranteed to be bug-free.

While many of the components of Sia are well understood and trusted
cryptographic ideas, Sia itself has not had a lot of academic review. As such,
Sia should be seen as highly experimental. As per the license, Sia is software
that comes without any warranty, and the developers cannot be held responsible
for any damages that occur. We encourage you to use Sia, but only with files
and money that you are comfortable losing.

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

- I mined a block, but I didn't receive any money.

  There is a 50-block confirmation delay before you will receive siacoins from
  mined blocks. If you still have not received the block reward after 50
  blocks, it means your block did not made it into the blockchain.

- I joined the network, but my block height is 0.

  Sia does not immediately synchronize upon joining the network. Please allow 5 minutes for synchronization to begin. Alternatively, you can force early synchronization by running `siac sync`.

- siad complains about "locks held too long."

  This is debugging output, and should not occur during normal use. Please
  contact us if this happens.

If your issue is not addressed above, you can get in touch with us personally:

  email:
  
  david@nebulouslabs.com
  
  luke@nebulouslabs.com
  
  IRC: #siacoin on freenode (ping Taek)

Version Information
-------------------

- If you intend to host files, you **must** forward your host port, and you
  should do so before making your host announcement. The default host port is
  9982.

- Please mine. Mining helps keep the network running smoothly. It can also
  cause changes to propagate if they seem to be taking a while.

- Uploading may take a long time, since the file contract needs to make it
  into a block. The default redundancy is also set very high, so uploading may
  be more expensive than expected.

Please tell us about any problems you run into, and any features you want! The
advantage of being a beta user is that your feedback will have a large impact
on what we do in the next few months. Thank you!

Version History
---------------

v0.3.1: Third open beta release.

v0.3.0: Second open beta release.

v0.2.0: First open beta release.

v0.1.0: Closed beta release.
