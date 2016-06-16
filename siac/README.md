Siac Usage
==========

`siac` is the command line interface to Sia, for use by power users and
those on headless servers. It comes as a part of the command line
package, and can be run as `./siac` from the same folder, or just by
calling `siac` if you move the binary into your path.

Most of the following commands have online help. For example, executing
`siac wallet send help` will list the arguments for that command,
while `siac host help` will list the commands that can be called
pertaining to hosting. `siac help` will list all of the top level
command groups that can be used.

You can change the address of where siad is pointing using the `-a`
flag. For example, `siac -a :9000 status` will display the status of
the siad instance launched on the local machine with `siad -a :9000`.

Common tasks
------------
* `siac status` view block height

Wallet:
* `siac wallet init [-p]` initilize a wallet
* `siac wallet unlock` unlock a wallet
* `siac wallet status` retrieve wallet balance
* `siac wallet address` get a wallet address
* `siac wallet send [amount] [dest]` sends siacoin to an address

Renter:
* `siac renter list` list all renter files
* `siac renter upload [filepath] [nickname]` upload a file
* `siac renter download [nickname] [filepath]` download a file
* `siac renter share [nickname] [filepath]` create a .sia file
* `siac renter load [filepath]` load a .sia file


Full Descriptions
-----------------

#### Wallet tasks

* `siac wallet init [-p]` encrypts and initializes the wallet. If the
`-p` flag is provided, an encryption password is requested from the
user. Otherwise the initial seed is used as the encryption
password. The wallet must be initialized and unlocked before any
actions can be performed on the wallet.

Examples:
```bash
user@hostname:~$ siac -a :9920 wallet init
Seed is:
 cider sailor incur sober feast unhappy mundane sadness hinder aglow imitate amaze duties arrow gigantic uttered inflamed girth myriad jittery hexagon nail lush reef sushi pastry southern inkling acquire

Wallet encrypted with password: cider sailor incur sober feast unhappy mundane sadness hinder aglow imitate amaze duties arrow gigantic uttered inflamed girth myriad jittery hexagon nail lush reef sushi pastry southern inkling acquire
```

```bash
user@hostname:~$ siac -a :9920 wallet init -p
Wallet password:
Seed is:
 potato haunted fuming lordship library vane fever powder zippers fabrics dexterity hoisting emails pebbles each vampire rockets irony summon sailor lemon vipers foxes oneself glide cylinder vehicle mews acoustic

Wallet encrypted with given password
```

* `siac wallet unlock` prompts the user for the encryption password
to the wallet, supplied by the `init` command. The wallet must be
initialized and unlocked before any actions can take place.

* `siac wallet status` prints information about your wallet.

Example:
```bash
user@hostname:~$ siac wallet status
Wallet status:
Encrypted, Unlocked
Confirmed Balance:   61516458.00 SC
Unconfirmed Balance: 64516461.00 SC
Exact:               61516457999999999999999999999999 H
```

* `siac wallet address` returns a never seen before address for sending
siacoins to.

* `siac wallet send [amount] [dest]` Sends `amount` siacoins to
`dest`. `amount` is in the form XXXXUU where an X is a number and U is
a unit, for example MS, S, mS, ps, etc. If no unit is given hastings
is assumed. `dest` must be a valid siacoin address.

* `siac wallet lock` locks a wallet. After calling, the wallet must be unlocked
using the encryption password in order to use it further

* `siac wallet seeds` returns the list of secret seeds in use by the
wallet. These can be used to regenerate the wallet

* `siac wallet addseed` prompts the user for his encryption password,
as well as a new secret seed. The wallet will then incorporate this
seed into itself. This can be used for wallet recovery and merging.

#### Host tasks
* `host config [setting] [value]`

is used to configure hosting.

| Setting      | Value                                            |
| ------------ | ------------------------------------------------ |
| totalstorage | The total size you will be hosting from in bytes |
| minfilesize  | The minimum file size you can host in bytes      |
| maxfilesize  | The maximum file size you can host in bytes      |
| minduration  | The smallest duration you can host for in blocks |
| maxduration  | The largest duration you can host for in blocks  |
| price        | Number of Siacoins per Gigabyte per month.       |

You can call this many times to configure you host before
announcing. Alternatively, you can manually adjust these parameters
inside the `host/config.json` file.

* `siac host announce` makes an host announcement. You may optionally
supply a specific address to be announced; this allows you to announce a domain
name. Announcing a second time after changing settings is not necessary, as the
announcement only contains enough information to reach your host.

* `siac host status` outputs some of your hosting settings.

Example:
```bash
user@hostname:~$ siac host status
Host settings:
Storage:      2.0000 TB (1.524 GB used)
Price:        0.000 SC per GB per month
Collateral:   0
Max Filesize: 10000000000
Max Duration: 8640
Contracts:    32
```

* `siac host hostdb` prints a list of all the know active hosts on the
network. It can also be called through `siac hostdb`

#### Renter tasks
* `siac renter upload [filename] [nickname]` uploads a file to the sia
network. `filename` is the path to the file you want to upload, and
nickname is what you will use to refer to that file in the
network. For example, it is common to have the nickname be the same as
the filename.

* `siac renter list` displays a list of the your uploaded files
currently on the sia network by nickname, and their filesizes.

* `siac renter download [nickname] [destination]` downloads a file
from the sia network onto your computer. `nickname` is the name used
to refer to your file in the sia network, and `destination` is the
path to where the file will be. If a file already exists there, it
will be overwritten.

* `siac renter rename [nickname] [newname]` changes the nickname of a
  file.

* `siac renter share [nickname] [filepath]` writes a .sia file
pointing to the file specified by `nickname` on the network. The file
is written to `filepath`. Note that the `.sia` extension will not be
automatically added, and must be part of the path.

* `siac renter shareascii [nickname]` writes the .sia file specified
  by `nickname` to stdout base64 encoded.

* `siac renter load [filename]` parses the .sia file at `filename` and
adds it to the renters collection of files, so that it can be
downloaded.

* `siac renter loadascii [data]` parses the siafile passed as an
argument and adds it to your collection of files for download. Data
will be a very large field.

* `siac renter delete [nickname]` removes a file from your list of
stored files. This does not remove it from the network, but only from
your saved list.

* `siac renter queue` shows the download queue. This is only relevant
if you have multiple downloads happening simultaneously.

#### Gateway tasks
* `siac gateway` prints info about the gateway, including its address and how
many peers it's connected to.

* `siac gateway list` prints a list of all currently connected peers.

* `siac gateway connect [address:port]` manually connects to a peer and adds it
to the gateway's node list.

* `siac gateway disconnect [address:port]` manually disconnects from a peer, but
leaves it in the gateway's node list.

#### Miner tasks
* `siac miner status` returns information about the miner. It is only
valid for when siad is running.

* `siac miner start` starts running the CPU miner on one thread. This
is virtually useless outside of debugging.

* `siac miner stop` halts the CPU miner.

#### General commands
* `siac status` prints the current block ID, current block height, and
current target.

* `siac stop` sends the stop signal to siad to safely terminate. This
has the same affect as C^c on the terminal.

* `siac version` displays the version string of siac.

* `siac update` checks the server for updates.
