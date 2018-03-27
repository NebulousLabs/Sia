Wallet
======

This document contains detailed descriptions of the wallet's API routes. For an
overview of the wallet's API routes, see [API.md#wallet](/doc/API.md#wallet).
For an overview of all API routes, see [API.md](/doc/API.md)

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Overview
--------

The wallet stores and manages siacoins and siafunds. The wallet's API endpoints
expose methods for creating and loading wallets, locking and unlocking, sending
siacoins and siafunds, and getting the wallet's balance.

You must create a wallet before you can use the wallet's API endpoints. You can
create a wallet with the `/wallet/init` endpoint. Wallets are always encrypted
on disk. Calls to some wallet API endpoints will fail until the wallet is
unlocked. The wallet can be unlocked with the `/wallet/unlock` endpoint. Once
the wallet is unlocked calls to the API endpoints will succeed until the wallet
is locked again with `/wallet/lock`, or Siad is restarted. The host and renter
require the miner to be unlocked.

Index
-----

| Route                                                           | HTTP verb |
| --------------------------------------------------------------- | --------- |
| [/wallet](#wallet-get)                                          | GET       |
| [/wallet/033x](#wallet033x-post)                                | POST      |
| [/wallet/address](#walletaddress-get)                           | GET       |
| [/wallet/addresses](#walletaddresses-get)                       | GET       |
| [/wallet/backup](#walletbackup-get)                             | GET       |
| [/wallet/changepassword](#walletchangepassword-post)            | POST      |
| [/wallet/init](#walletinit-post)                                | POST      |
| [/wallet/init/seed](#walletinitseed-post)                       | POST      |
| [/wallet/lock](#walletlock-post)                                | POST      |
| [/wallet/seed](#walletseed-post)                                | POST      |
| [/wallet/seeds](#walletseeds-get)                               | GET       |
| [/wallet/siacoins](#walletsiacoins-post)                        | POST      |
| [/wallet/siafunds](#walletsiafunds-post)                        | POST      |
| [/wallet/siagkey](#walletsiagkey-post)                          | POST      |
| [/wallet/sign](#walletsign-post)                                | POST      |
| [/wallet/sweep/seed](#walletsweepseed-post)                     | POST      |
| [/wallet/transaction/___:id___](#wallettransactionid-get)       | GET       |
| [/wallet/transactions](#wallettransactions-get)                 | GET       |
| [/wallet/transactions/___:addr___](#wallettransactionsaddr-get) | GET       |
| [/wallet/unlock](#walletunlock-post)                            | POST      |
| [/wallet/unspent](#walletunspent-get)                           | GET       |
| [/wallet/verify/address/:___addr___](#walletverifyaddress-get)  | GET       |

#### /wallet [GET]

returns basic information about the wallet, such as whether the wallet is
locked or unlocked.

###### JSON Response
```javascript
{
  // Indicates whether the wallet has been encrypted or not. If the wallet
  // has not been encrypted, then no data has been generated at all, and the
  // first time the wallet is unlocked, the password given will be used as
  // the password for encrypting all of the data. 'encrypted' will only be
  // set to false if the wallet has never been unlocked before (the unlocked
  // wallet is still encryped - but the encryption key is in memory).
  "encrypted": true,

  // Indicates whether the wallet is currently locked or unlocked. Some calls
  // become unavailable when the wallet is locked.
  "unlocked": true,

  // Indicates whether the wallet is currently rescanning the blockchain. This
  // will be true for the duration of calls to /unlock, /seeds, /init/seed,
  // and /sweep/seed.
  "rescanning": false,

  // Number of siacoins, in hastings, available to the wallet as of the most
  // recent block in the blockchain.
  "confirmedsiacoinbalance": "123456", // hastings, big int

  // Number of siacoins, in hastings, that are leaving the wallet according
  // to the set of unconfirmed transactions. Often this number appears
  // inflated, because outputs are frequently larger than the number of coins
  // being sent, and there is a refund. These coins are counted as outgoing,
  // and the refund is counted as incoming. The difference in balance can be
  // calculated using 'unconfirmedincomingsiacoins' - 'unconfirmedoutgoingsiacoins'
  "unconfirmedoutgoingsiacoins": "0", // hastings, big int

  // Number of siacoins, in hastings, are entering the wallet according to
  // the set of unconfirmed transactions. This number is often inflated by
  // outgoing siacoins, because outputs are frequently larger than the amount
  // being sent. The refund will be included in the unconfirmed incoming
  // siacoins balance.
  "unconfirmedincomingsiacoins": "789", // hastings, big int

  // Number of siafunds available to the wallet as of the most recent block
  // in the blockchain.
  "siafundbalance": "1", // big int

  // Number of siacoins, in hastings, that can be claimed from the siafunds
  // as of the most recent block. Because the claim balance increases every
  // time a file contract is created, it is possible that the balance will
  // increase before any claim transaction is confirmed.
  "siacoinclaimbalance": "9001", // hastings, big int

  // Number of siacoins, in hastings per byte, below which a transaction output
  // cannot be used because the wallet considers it a dust output
  "dustthreshold": "1234", // hastings / byte, big int
}
```

#### /wallet/033x [POST]

loads a v0.3.3.x wallet into the current wallet, harvesting all of the secret
keys. All spendable addresses in the loaded wallet will become spendable from
the current wallet. An error will be returned if the given `encryptionpassword`
is incorrect.

###### Query String Parameters
```
// Path on disk to the v0.3.3.x wallet to be loaded.
source

// Encryption key of the wallet.
encryptionpassword
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /wallet/address [GET]

gets a new address from the wallet generated by the primary seed. An error will
be returned if the wallet is locked.

###### JSON Response
```javascript
{
  // Wallet address that can receive siacoins or siafunds. Addresses are 76 character long hex strings.
  "address": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab"
}
```

#### /wallet/addresses [GET]

fetches the list of addresses from the wallet. If the wallet has not been
created or unlocked, no addresses will be returned. After the wallet is
unlocked, this call will continue to return its addresses even after the
wallet is locked again.

###### JSON Response
```javascript
{
  // Array of wallet addresses owned by the wallet.
  "addresses": [
    "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  ]
}
```

#### /wallet/backup [GET]

creates a backup of the wallet settings file. Though this can easily be done
manually, the settings file is often in an unknown or difficult to find
location. The /wallet/backup call can spare users the trouble of needing to
find their wallet file. The destination file is overwritten if it already
exists.

###### Query String Parameters
```
// path to the location on disk where the backup file will be saved.
destination
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /wallet/changepassword [POST]

changes the wallet's encryption password.

###### Query String Parameter
```
// encryptionpassword is the wallet's current encryption password.
encryptionpassword
// newpassword is the new password for the wallet.
newpassword
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

#### /wallet/init [POST]

initializes the wallet. After the wallet has been initialized once, it does not
need to be initialized again, and future calls to /wallet/init will return an
error, unless the force flag is set. The encryption password is provided by the
api call. If the password is blank, then the password will be set to the same
as the seed.

###### Query String Parameters
```
// Password that will be used to encrypt the wallet. All subsequent calls
// should use this password. If left blank, the seed that gets returned will
// also be the encryption password.
encryptionpassword

// Name of the dictionary that should be used when encoding the seed. 'english'
// is the most common choice when picking a dictionary.
dictionary // Optional, default is english.

// boolean, when set to true /wallet/init will Reset the wallet if one exists
// instead of returning an error. This allows API callers to reinitialize a new
// wallet.
force
```

###### JSON Response
```javascript
{
  // Wallet seed used to generate addresses that the wallet is able to spend.
  "primaryseed": "hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello"
}
```

#### /wallet/init/seed [POST]

initializes the wallet using a preexisting seed. After the wallet has been
initialized once, it does not need to be initialized again, and future calls to
/wallet/init/seed will return an error unless the force flag is set. The
encryption password is provided by the api call. If the password is blank, then
the password will be set to the same as the seed. Note that loading a
preexisting seed requires scanning the blockchain to determine how many keys
have been generated from the seed.  For this reason, /wallet/init/seed can only
be called if the blockchain is synced.

###### Query String Parameters
```
// Password that will be used to encrypt the wallet. All subsequent calls
// should use this password. If left blank, the seed that gets returned will
// also be the encryption password.
encryptionpassword

// Name of the dictionary that should be used when encoding the seed. 'english'
// is the most common choice when picking a dictionary.
dictionary // Optional, default is english.

// Dictionary-encoded phrase that corresponds to the seed being used to
// initialize the wallet.
seed

// boolean, when set to true /wallet/init will Reset the wallet if one exists
// instead of returning an error. This allows API callers to reinitialize a new
// wallet.
force
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /wallet/seed [POST]

gives the wallet a seed to track when looking for incoming transactions. The
wallet will be able to spend outputs related to addresses created by the seed.
The seed is added as an auxiliary seed, and does not replace the primary seed.
Only the primary seed will be used for generating new addresses.

###### Query String Parameters
```
// Key used to encrypt the new seed when it is saved to disk.
encryptionpassword

// Name of the dictionary that should be used when encoding the seed. 'english'
// is the most common choice when picking a dictionary.
dictionary

// Dictionary-encoded phrase that corresponds to the seed being added to the
// wallet.
seed
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /wallet/seeds [GET]

returns a list of seeds in use by the wallet. The primary seed is the only seed
that gets used to generate new addresses. This call is unavailable when the
wallet is locked.

A seed is an encoded version of a 128 bit random seed. The output is 15 words
chosen from a small dictionary as indicated by the input. The most common
choice for the dictionary is going to be 'english'. The underlying seed is the
same no matter what dictionary is used for the encoding. The encoding also
contains a small checksum of the seed, to help catch simple mistakes when
copying. The library
[entropy-mnemonics](https://github.com/NebulousLabs/entropy-mnemonics) is used
when encoding.

###### Query String Parameters
```
// Name of the dictionary that should be used when encoding the seed. 'english'
// is the most common choice when picking a dictionary.
dictionary
```

###### JSON Response
```javascript
{
  // Seed that is actively being used to generate new addresses for the wallet.
  "primaryseed": "hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello",

  // Number of addresses that remain in the primary seed until exhaustion has
  // been reached. Once exhaustion has been reached, new addresses will
  // continue to be generated but they will be more difficult to recover in the
  // event of a lost wallet file or encryption password.
  "addressesremaining": 2500,

  // Array of all seeds that the wallet references when scanning the blockchain
  // for outputs. The wallet is able to spend any output generated by any of
  // the seeds, however only the primary seed is being used to generate new
  // addresses.
  "allseeds": [
    "hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello",
    "foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo bar foo",
  ]
}
```

#### /wallet/siacoins [POST]

Function: Send siacoins to an address or set of addresses. The outputs are
arbitrarily selected from addresses in the wallet. If 'outputs' is supplied,
'amount' and 'destination' must be empty. The number of outputs should not
exceed 400; this may result in a transaction too large to fit in the
transaction pool.

###### Query String Parameters
```
// Number of hastings being sent. A hasting is the smallest unit in Sia. There
// are 10^24 hastings in a siacoin.
amount      // hastings

// Address that is receiving the coins.
destination // address

// JSON array of outputs. The structure of each output is:
// {"unlockhash": "<destination>", "value": "<amount>"}
outputs
```

###### JSON Response
```javascript
{
  // Array of IDs of the transactions that were created when sending the coins.
  // The last transaction contains the output headed to the 'destination'.
  // Transaction IDs are 64 character long hex strings.
  transactionids [
    "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  ]
}
```

### Examples

#### Send to single address

###### Example POST Request
Use _amount_ and _destination_ parameters.
```
/wallet/siacoins?amount=1000000000000000000000000&destination=1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab
```

###### Expected Response Code
```
200 OK
```

###### Example Response Body
```json
{
  "transactionids": [
    "3918e4a4b4cee46b3e5b28b8a1cc41c064a6f6002d162d396f296c201e6edc13",
    "18b85b7d20f8a87bdadacf11e135ad44db1d93efd0613d23116e8cf255502762"
  ]
}
```


#### Send to set of addresses
Use _outputs_ parameter in the form of a JSON array. _amount_ and _destination_ parameters must be empty.


###### Example POST Request
```
/wallet/siacoins?outputs=[{"unlockhash":"1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab","value":"1000000000000000000000000"},{"unlockhash":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab1234567890","value":"8000000000000000000000000"},{"unlockhash":"cdef0123456789abcdef0123456789abcdef0123456789ab1234567890abcdef0123456789ab","value":"5000000000000000000000000"}]
```

###### (sample JSON request body for reference)
```json
{
  "outputs": [
    {
      "unlockhash":
"1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
      "value": "1000000000000000000000000"
    },
    {
      "unlockhash":
"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab1234567890",
      "value": "8000000000000000000000000"
    },
    {
      "unlockhash":
"cdef0123456789abcdef0123456789abcdef0123456789ab1234567890abcdef0123456789ab",
      "value": "20000000000000000000000000"
    }
  ]
}

```

###### Expected Response Code
```
200 OK
```

###### Example Response Body
```json
{
  "transactionids": [
    "21962e0118f3ca5d6fab0262c65bca0220fbcc828c499974d86e7cc4047a0ce5",
    "f2471d550823f2c0616565d8476a7fea5f2b9a841612bf109923c3a54e760721"
  ]
}
```

#### /wallet/siafunds [POST]

sends siafunds to an address. The outputs are arbitrarily selected from
addresses in the wallet. Any siacoins available in the siafunds being sent (as
well as the siacoins available in any siafunds that end up in a refund address)
will become available to the wallet as siacoins after 144 confirmations. To
access all of the siacoins in the siacoin claim balance, send all of the
siafunds to an address in your control (this will give you all the siacoins,
while still letting you control the siafunds).

###### Query String Parameters
```
// Number of siafunds being sent.
amount      // siafunds

// Address that is receiving the funds.
destination // address
```

###### JSON Response
```javascript
{
  // Array of IDs of the transactions that were created when sending the coins.
  // The last transaction contains the output headed to the 'destination'.
  // Transaction IDs are 64 character long hex strings.
  "transactionids": [
    "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  ]
}
```

#### /wallet/siagkey [POST]

Function: Load a key into the wallet that was generated by siag. Most siafunds
are currently in addresses created by siag.

###### Query String Parameters
```
// Key that is used to encrypt the siag key when it is imported to the wallet.
encryptionpassword

// List of filepaths that point to the keyfiles that make up the siag key.
// There should be at least one keyfile per required signature. The filenames
// need to be commna separated (no spaces), which means filepaths that contain
// a comma are not allowed.
keyfiles
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /wallet/sign [POST]

Function: Sign a transaction. The wallet will attempt to sign any SiacoinInput
in the transaction whose UnlockConditions are unset.

###### Query String Parameters
```
// base64-encoded transaction to be signed
transaction string
```

###### Response
```javascript
{
  // raw, base64 encoded transaction data
  "transaction": "AQAAAAAAAADBM1ca",

  // indices of inputs that were signed
  "signedinputs": [0, 1, 6]
}
```

#### /wallet/sweep/seed [POST]

Function: Scan the blockchain for outputs belonging to a seed and send them to
an address owned by the wallet.

###### Query String Parameters
```
// Name of the dictionary that should be used when decoding the seed. 'english'
// is the most common choice when picking a dictionary.
dictionary // Optional, default is english.

// Dictionary-encoded phrase that corresponds to the seed being added to the
// wallet.
seed
```

###### JSON Response
```javascript
{
  // Number of siacoins, in hastings, transferred to the wallet as a result of
  // the sweep.
  "coins": "123456", // hastings, big int

  // Number of siafunds transferred to the wallet as a result of the sweep.
  "funds": "1", // siafunds, big int
}
```

#### /wallet/lock [POST]

locks the wallet, wiping all secret keys. After being locked, the keys are
encrypted. Queries for the seed, to send siafunds, and related queries become
unavailable. Queries concerning transaction history and balance are still
available.

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /wallet/transaction/___:id___ [GET]

gets the transaction associated with a specific transaction id.

###### Path Parameters
```
// ID of the transaction being requested.
:id
```

###### JSON Response
```javascript
{
  "transaction": {
    // Raw transaction. The rest of the fields in the resposne are determined
    // from this raw transaction. It is left undocumented here as the processed
    // transaction (the rest of the fields in this object) are usually what is
    // desired.
    "transaction": {
      // See types.Transaction in https://github.com/NebulousLabs/Sia/blob/master/types/transactions.go
    },

    // ID of the transaction from which the wallet transaction was derived.
    "transactionid": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",

    // Block height at which the transaction was confirmed. If the transaction
    // is unconfirmed the height will be the max value of an unsigned 64-bit
    // integer.
    "confirmationheight": 50000,

    // Time, in unix time, at which a transaction was confirmed. If the
    // transaction is unconfirmed the timestamp will be the max value of an
    // unsigned 64-bit integer.
    "confirmationtimestamp": 1257894000,

    // Array of processed inputs detailing the inputs to the transaction.
    "inputs": [
      {
        // The id of the output being spent.
        "parentid": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",

        // Type of fund represented by the input. Possible values are
        // 'siacoin input' and 'siafund input'.
        "fundtype": "siacoin input",

        // true if the address is owned by the wallet.
        "walletaddress": false,

        // Address that is affected. For inputs (outgoing money), the related
        // address is usually not important because the wallet arbitrarily
        // selects which addresses will fund a transaction.
        "relatedaddress": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",

        // Amount of funds that have been moved in the input.
        "value": "1234", // hastings or siafunds, depending on fundtype, big int
      }
    ],
    // Array of processed outputs detailing the outputs of the transaction.
    // Outputs related to file contracts are excluded.
    "outputs": [
      {
        // The id of the output that was created.
        "id": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",

        // Type of fund is represented by the output. Possible values are
        // 'siacoin output', 'siafund output', 'claim output', and 'miner
        // payout'. Siacoin outputs and claim outputs both relate to siacoins.
        // Siafund outputs relate to siafunds. Miner payouts point to siacoins
        // that have been spent on a miner payout. Because the destination of
        // the miner payout is determined by the block and not the transaction,
        // the data 'maturityheight', 'walletaddress', and 'relatedaddress' are
        // left blank.
        "fundtype": "siacoin output",

        // Block height the output becomes available to be spent. Siacoin
        // outputs and siafund outputs mature immediately - their maturity
        // height will always be the confirmation height of the transaction.
        // Claim outputs cannot be spent until they have had 144 confirmations,
        // thus the maturity height of a claim output will always be 144 larger
        // than the confirmation height of the transaction.
        "maturityheight": 50000,

        // true if the address is owned by the wallet.
        "walletaddress": false,

        // Address that is affected. For outputs (incoming money), the related
        // address field can be used to determine who has sent money to the
        // wallet.
        "relatedaddress": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",

        // Amount of funds that have been moved in the output.
        "value": "1234", // hastings or siafunds, depending on fundtype, big int
      }
    ]
  }
}
```

#### /wallet/transactions [GET]

returns a list of transactions related to the wallet.

###### Query String Parameters
```
// Height of the block where transaction history should begin.
startheight // block height

// Height of of the block where the transaction history should end. If
// 'endheight' is greater than the current height, all transactions up to and
// including the most recent block will be provided.
endheight // block height
```

###### JSON Response
```javascript
{
  // All of the confirmed transactions appearing between height 'startheight'
  // and height 'endheight' (inclusive).
  "confirmedtransactions": [
    {
      // See the documentation for '/wallet/transaction/:id' for more information.
    }
  ],

  // All of the unconfirmed transactions.
  "unconfirmedtransactions": [
    {
      // See the documentation for '/wallet/transaction/:id' for more information.
    }
  ]
}
```

#### /wallet/transactions/___:addr___ [GET]

returns all of the transactions related to a specific address.

###### Path Parameters
```
// Unlock hash (i.e. wallet address) whose transactions are being requested.
:addr
```

###### JSON Response
```javascript
{
  // Array of processed transactions that relate to the supplied address.
  "transactions": [
    {
      // See the documentation for '/wallet/transaction/:id' for more information.
    }
  ]
}
```

#### /wallet/unlock [POST]

unlocks the wallet. The wallet is capable of knowing whether the correct
password was provided.

###### Query String Parameters
```
// Password that gets used to decrypt the file. Most frequently, the encryption
// password is the same as the primary wallet seed.
encryptionpassword string
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /wallet/unspent [GET]

returns a list of outputs that the wallet can spend.

###### Response
```javascript
{
  // Array of outputs that the wallet can spend.
  "outputs": [
    {
      // The id of the output.
      "id": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",

      // Type of output, either 'siacoin output' or 'siafund output'.
      "fundtype": "siacoin output",

      // Height of block in which the output appeared. To calculate the
      // number of confirmations, subtract this number from the current
      // block height.
      "maturityheight": 50000,

      // Irrelevant field shared by ProcessedOutput; always true.
      "walletaddress": true,

      // UnlockHash of the output.
      "relatedaddress": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",

      // Amount of funds in the output; hastings for siacoin outputs, and
      // siafunds for siafund outputs.
      "value": "1234" // big int
    }
  ]
}
```

#### /wallet/verify/address/:addr [GET]

takes the address specified by :addr and returns a JSON response indicating if the address is valid.

###### JSON Response
```javascript
{
	// valid indicates if the address supplied to :addr is a valid UnlockHash.
	"valid": true
}
```
