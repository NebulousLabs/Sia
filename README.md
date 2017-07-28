# [![Sia Logo](http://sia.tech/img/svg/sia-green-logo.svg)](http://sia.tech) v1.3.0 (Capricorn)

[![Build Status](https://travis-ci.org/NebulousLabs/Sia.svg?branch=master)](https://travis-ci.org/NebulousLabs/Sia)
[![GoDoc](https://godoc.org/github.com/NebulousLabs/Sia?status.svg)](https://godoc.org/github.com/NebulousLabs/Sia)
[![Go Report Card](https://goreportcard.com/badge/github.com/NebulousLabs/Sia)](https://goreportcard.com/report/github.com/NebulousLabs/Sia)

Sia is a new decentralized cloud storage platform that radically alters the
landscape of cloud storage. By leveraging smart contracts, client-side
encryption, and sophisticated redundancy (via Reed-Solomon codes), Sia allows
users to safely store their data with hosts that they do not know or trust.
The result is a cloud storage marketplace where hosts compete to offer the
best service at the lowest price. And since there is no barrier to entry for
hosts, anyone with spare storage capacity can join the network and start
making money.

![UI](http://i.imgur.com/iHoGqoL.png)

Traditional cloud storage has a number of shortcomings. Users are limited to a
few big-name offerings: Google, Microsoft, Amazon. These companies have little
incentive to encrypt your data or make it easy to switch services later. Their
code is closed-source, and they can lock you out of your account at any time.

We believe that users should own their data. Sia achieves this by replacing
the traditional monolithic cloud storage provider with a blockchain and a
swarm of hosts, each of which stores an encrypted fragment of your data. Since
the fragments are redundant, no single host can hold your data hostage: if
they jack up their price or go offline, you can simply download from a
different host. In other words, trust is removed from the equation, and
switching to a different host is painless. Stripped of these unfair
advantages, hosts must compete solely on the quality and price of the storage
they provide.

Sia can serve as a replacement for personal backups, bulk archiving, content
distribution, and more. For developers, Sia is a low-cost alternative to
Amazon S3. Storage on Sia is a full order of magnitude cheaper than on S3,
with comparable bandwidth, latency, and durability. Sia works best for static
content, especially media like videos, music, and photos.

Distributing data across many hosts automatically confers several advantages.
The most obvious is that, just like BitTorrent, uploads and downloads are
highly parallel. Given enough hosts, Sia can saturate your bandwidth. Another
advantage is that your data is spread across a wide geographic area, reducing
latency and safeguarding your data against a range of attacks.

It is important to note that users have full control over which hosts they
use. You can tailor your host set for minimum latency, lowest price, widest
geographic coverage, or even a strict whitelist of IP addresses or public
keys.

At the core of Sia is a blockchain that closely resembles Bitcoin.
Transactions are conducted in Siacoin, a cryptocurrency. The blockchain is
what allows Sia to enforce its smart contracts without relying on centralized
authority. To acquire siacoins, use an exchange such as [Bittrex](https://bittrex.com), [Yunbi](https://yunbi.com), or
[Bisq](https://www.bisq.io).

To get started with Sia, check out the guides below:

- [How to Store Data on Sia](https://blog.sia.tech/getting-started-with-private-decentralized-cloud-storage-c9565dc8c854)
- [How to Become a Sia Host](https://blog.sia.tech/how-to-run-a-host-on-sia-2159ebc4725)
- [Using the Sia API](https://blog.sia.tech/api-quickstart-guide-f1d160c05235)


Usage
-----

Sia is ready for use with small sums of money and non-critical files, but
until the network has a more proven track record, we advise against using it
as a sole means of storing important data.

This release comes with 2 binaries, siad and siac. siad is a background
service, or "daemon," that runs the Sia protocol and exposes an HTTP API on
port 9980. siac is a command-line client that can be used to interact with
siad in a user-friendly way. There is also a graphical client, [Sia-UI](https://github.com/NebulousLabs/Sia-UI), which
is the preferred way of using Sia for most users. For interested developers,
the siad API is documented [here](doc/API.md).

siad and siac are run via command prompt. On Windows, you can just double-
click siad.exe if you don't need to specify any command-line arguments.
Otherwise, navigate to its containing folder and click File->Open command
prompt. Then, start the siad service by entering `siad` and pressing Enter.
The command prompt may appear to freeze; this means siad is waiting for
requests. Windows users may see a warning from the Windows Firewall; be sure
to check both boxes ("Private networks" and "Public networks") and click
"Allow access." You can now run `siac` (in a separate command prompt) or Sia-
UI to interact with siad. From here, you can send money, upload and download
files, and advertise yourself as a host.

Building From Source
--------------------

To build from source, [Go 1.8 must be installed](https://golang.org/doc/install)
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
