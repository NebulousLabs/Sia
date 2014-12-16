Sia 0.1.0
=========

This distribution is an early beta release. It is likely to have many bugs,
some of which may be severe. Please use with caution.

This release comes with 2 binaries, siad and siac. siad is a background
service, or "daemon", that runs the Sia protocol, and siac is a client that is
used to interact with siad. siad also exposes a web interface, which can be
viewed in your browser at `http://localhost:9980` while siad is running. This is the
preferred way of interacting with siad.

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
siad, or navigate your browser to `http://localhost:9980` to use siad's web
interface. From here, you can send money, mine blocks, upload and download
files, and advertise yourself as a host.

A note on hosting: when you announce yourself as a host, you have to put up
coins ("freeze" them) to show that you're serious. This helps to mitiage Sybil
attacks. After 'freeze duration' blocks, you will get the coins back.

When someone makes a contract with you, you put up security coins. If you're a
good host, you'll eventually get these coins back, but if you lose the file
you'll lose the security too. So when people make contracts with you, your
balance will initially go down. Rest assured, once you start submitting storage
proofs, you'll start making money.

Troubleshooting
---------------

- siad prints `Can't bootstrap: no peers responded to ping.`

  There should be at least one peer online for you to connect to, so if you
  see this message you may be experiencing connection issues. Ensure that you
  are connected to the Internet. You may also need to forward your ports (see
  below). If you are confident that your connection is okay, contact us! Our
  server may be experiencing problems.

  You can also opt not to bootstrap at all by passing a "-n" flag to siad.

  Port forwarding:
  Port forwarding is how you let your router know that it's okay for other Sia
  peers to connect to you. If you are behind a firewall, you will most likely
  need to do this. By default, Sia traffic happens on port 9988. The specific
  instructions for forwarding a port varies by router. Consult this guide for
  more information: portfoward.com.

  In future versions, we will add support for UPnP, which will allow you to
  skip this step.

If your issue is not addressed above, you can get in touch with us personally:
  email: david@nebulouslabs.com
         luke@nebulouslabs.com
  IRC:   #siacoin on freenode

Version Information
-------------------

- siad starts fresh every time you run it. When you close it, everything is
  lost: your wallet, your coins, your files, the blockchain, everything. So
  for the sake of other beta users, please leave siad running as long as
  possible. And don't upload anything important!

- Hosts don't keep track of coins they've frozen. They are lost forever. Don't
  worry, beta coins are worthless anyway.

- Files are only stored for 500 blocks (about 3 days). There is no way to
  modify this. Future versions will make this a configurable setting.

- Some elements of the web interface update in real time, but not all of them.
  Most noticeably, on the Overview page, your block height will update in real
  time, but your wallet balance will not. Simply click "Overview" again to
  refresh your balance.

- If you did not bootstrap, you need to manually enter your IP in order to
  become a host. It will read `:9988`; change this to `your.ip.goes.here:9988`

Please tell us about any problems you run into, and any features you want! The
advantage of being a beta user is that your feedback will have a large impact
on what we do in the next few months. Thank you!

Version History
---------------

v0.1.0: Initial (closed) beta release.
