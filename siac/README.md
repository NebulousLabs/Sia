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

Common tasks
------------
* `siac wallet init [-p]` initilize a wallet
* `siac wallet unlock` unlock a wallet
* `siac wallet status` retrieve wallet balance
* `siac wallet address` get a wallet address
* `siac wallet send [amount] [dest]` sends siacoin to an address

Full Descriptions
-----------------

#### Wallet tasks

`siac wallet init [-p]` encrypts and initializes the wallet. If the
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

`siac wallet unlock` prompts the user for the encryption password
to the wallet, supplied by the `init` command. The wallet must be
initialized and unlocked before any actions can take place.

`siac wallet status` prints information about your wallet.

Example:
```bash
user@hostname:~$ siac wallet status
Wallet status:
Encrypted, Unlocked
Confirmed Balance:   61516458.00 SC
Unconfirmed Balance: 64516461.00 SC
Exact:               61516457999999999999999999999999 H
```

`siac wallet address` returns a never seen before address for sending
siacoins to.

`siac wallet send [amount] [dest]` Sends `amount` siacoins to
`dest`. `amount` is in the form XXXXUU where an X is a number and U is
a unit, for example MS, S, mS, ps, etc. If no unit is given hastings
is assumed. `dest` must be a valid siacoin address.

`siac wallet lock` locks a wallet. After calling, the wallet must be unlocked using the
encryption password in order to use it further

`siac wallet seeds` returns the list of secret seeds in use by the
wallet. These can be used to regenerate the wallet

`siac wallet addseed` prompts the user for his encryption password,
as well as a new secret seed. The wallet will then incorporate this
seed into itself. This can be used for wallet recovery and merging.

#### Host tasks
`host config [setting] [value]` is used to configure hosting.

| Setting      | Value                                            |
| ------------ | ------------------------------------------------ |
| totalstorage | The total size you will be hosting from in bytes |
| minfilisize  | The minimum file size you can host in bytes      |
| maxfilesize  | The maximum file size you can host in bytes      |
| minduration  | The smallest duration you can host for in blocks |
| maxduration  | The largest duration you can host for in blocks  |
| windowsize   |                                                  |
| price        | Number of Siacoins per Gigabyte per month.       |

You can call this many times to configure you host before
announcing. Alternatively, you can manually adjust these parameters
inside the `host/config.json` file.

`siac host announce [-f]` makes an host announcement. If the `-f` flag
is passed, it will force the announcement. Otherwise, you cannot
annonuce multiple times. Announcing a second time after changing
settings is not necessary, as the announcement only contains enough
information to reach your host.

`siac host status` outputs some of your hosting settings.

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

`siac host hostdb` prints a list of all the know active hosts on the
network. It can also be called through `siac hostdb`

#### Renter tasks
