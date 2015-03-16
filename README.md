Sia 0.3.0
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
used to interact with siad. Siad exposes an api on 'localhost:9980' which can
be used to interact with the daemon. There is a front-end program called Sia-UI
which can be used to interact with the daemon in a more user-friendly way.
Documentation on the API can be found in API.md.

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
  are connected to the Internet. You may also need to forward your ports (see
  below). If you are confident that your connection is okay, contact us! Our
  server may be experiencing problems.

  You can also opt not to bootstrap at all by passing a "-n" flag to siad.

  Port forwarding:
  Port forwarding is how you let your router know that it's okay for other Sia
  peers to connect to you. If you are behind a firewall, you will most likely
  need to do this. By default, Sia traffic happens on port 9988. The specific
  instructions for forwarding a port varies by router. For more information,
  consult [this guide](http://portfoward.com).

  In future versions, we will add support for UPnP, which will allow you to
  skip this step.

- The block height has stopped increasing.

  This is usually caused by a minor desynchronization. The client should
  automatically attempt to resynchronize every two minutes. You can also
  manually resynchronize by running `siac gateway sync`.

If your issue is not addressed above, you can get in touch with us personally:

  email:
  
  david@nebulouslabs.com
  
  luke@nebulouslabs.com
  
  IRC: #siacoin on freenode (ping Taek)

Version Information
-------------------

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

v0.3.0: Second open beta release.

v0.2.0: First open beta release.

v0.1.0: Closed beta release.
