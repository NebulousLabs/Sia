Sia Address Generator 1.0
=========================

The Sia address generator can be used to create M-of-N multisig Sia addresses.
N keyfiles will be created, each containing a secret key (also called a private
key). M of these files will be necessary to spend siacoins or siafunds sent to
the address.

To create an address, run `siag` in a command prompt or terminal. Files will be
created in whatever folder the command prompt is in. Keep these files in a safe
place, if you lose the files you will not be able to spend assets sent to your
address.

The default will create a 1-of-1 address, which is a normal address. One
keyfile will be created. To create a 2-of-3 address, run `siag -r 2 -t 3`.
Three keyfiles will be created, and two are required to spend any of the assets
sent to the address.

Finally, you can print information about a key by running `siag keyinfo
myKeyFile.siakey`. This will print the address that the key unlocks, as well as
the multisig settings of the key.
