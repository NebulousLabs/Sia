Sia 0.1.0
=========

This distribution is an early beta release. It is likely to have many bugs,
some of which may be severe. Please use with caution.

This release comes with 2 binaries, siad and siac. siad is a background
service, or "daemon", that runs the Sia protocol, and siac is a client that is
used to interact with siad. siad also exposes a web interface, which can be
viewed in your browser at 'localhost:9980' while siad is running. This is the
preferred way of interacting with siad.

Usage
-----

siad and siac are run via command prompt. On Windows, you can open a command
prompt by navigating to the sia folder and clicking File->Open Command Window.
Then, start the siad service by entering 'siad' and pressing Enter. The command
prompt may appear to freeze; this means siad is waiting for requests. You can
now run 'siac' in a separate command prompt to interact with siad, or navigate
your browser to 'localhost:9980' to use siad's web interface. From here, you
can send money, mine blocks, and upload and download files.

You can also advertise yourself as a host. This process is currently a bit
unintuitive. When you become a host, you have to put up coins ("freeze" them)
to show that you're serious. If you're a good host, you'll eventually get
these coins back, but if you lose files you'll lose the coins too. So when
people make contracts with you, your balance will initially go down. Rest
assured, once you start submitting storage proofs, you'll start making money.

Version Information
-------------------

v0.1.0:

- siad starts fresh every time you run it. When you close it, everything is
  lost: your wallet, your coins, your files, the blockchain, everything. So
  for the sake of other beta users, please leave siad running as long as
  possible. And don't upload anything important!

- Hosts don't keep track of coins they've frozen. They are lost forever. Don't
  worry, beta coins are worthless anyway.

Please tell us about any problems you run into, and any features you want! The
advantage of being a beta user is that your feedback will have a large impact
on what we do in the next few months. Thank you!
