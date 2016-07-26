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
on disk. Calls to any wallet API endpoints will fail until the wallet is
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
| [/wallet/init](#walletinit-post)                                | POST      |
| [/wallet/lock](#walletlock-post)                                | POST      |
| [/wallet/seed](#walletseed-post)                                | POST      |
| [/wallet/seeds](#walletseeds-get)                               | GET       |
| [/wallet/siacoins](#walletsiacoins-post)                        | POST      |
| [/wallet/siafunds](#walletsiafunds-post)                        | POST      |
| [/wallet/siagkey](#walletsiagkey-post)                          | POST      |
| [/wallet/transaction/___:id___](#wallettransactionid-get)       | GET       |
| [/wallet/transactions](#wallettransactions-get)                 | GET       |
| [/wallet/transactions/___:addr___](#wallettransactionsaddr-get) | GET       |
| [/wallet/unlock](#walletunlock-post)                            | POST      |

#### /wallet [GET]

Function: Returns basic information about the wallet, such as whether the
wallet is locked or unlocked.

Parameters: none

Response:
```
struct {
	encrypted bool
	unlocked  bool

	confirmedsiacoinbalance     types.Currency (string)
	unconfirmedoutgoingsiacoins types.Currency (string)
	unconfirmedincomingsiacoins types.Currency (string)

	siafundbalance      types.Currency (string)
	siacoinclaimbalance types.Currency (string)
}
```
'encrypted' indicates whether the wallet has been encrypted or not. If the
wallet has not been encrypted, then no data has been generated at all, and the
first time the wallet is unlocked, the password given will be used as the
password for encrypting all of the data. 'encrypted' will only be set to false
if the wallet has never been unlocked before (the unlocked wallet is still
encryped - but the encryption key is in memory).

'unlocked' indicates whether the wallet is currently locked or unlocked. Some
calls become unavailable when the wallet is locked.

'confirmedsiacoinbalance' is the number of siacoins available to the wallet as
of the most recent block in the blockchain.

'unconfirmedoutgoingsiacoins' is the number of siacoins that are leaving the
wallet according to the set of unconfirmed transactions. Often this number
appears inflated, because outputs are frequently larger than the number of
coins being sent, and there is a refund. These coins are counted as outgoing,
and the refund is counted as incoming. The difference in balance can be
calculated using 'unconfirmedincomingsiacoins' - 'unconfirmedoutgoingsiacoins'

'unconfirmedincomingsiacoins' is the number of siacoins are entering the wallet
according to the set of unconfirmed transactions. This number is often inflated
by outgoing siacoins, because outputs are frequently larger than the amount
being sent. The refund will be included in the unconfirmed incoming siacoins
balance.

'siafundbalance' is the number of siafunds available to the wallet as
of the most recent block in the blockchain.

'siacoinclaimbalance' is the number of siacoins that can be claimed from the
siafunds as of the most recent block. Because the claim balance increases every
time a file contract is created, it is possible that the balance will increase
before any claim transaction is confirmed.

#### /wallet/033x [POST]

Function: Load a v0.3.3.x wallet into the current wallet, harvesting all of the
secret keys. All spendable addresses in the loaded wallet will become spendable
from the current wallet.

Parameters:
```
source             string
encryptionpassword string
```
'source' is the location on disk of the v0.3.3.x wallet that is being loaded.

'encryptionpassword' is the encryption key of the wallet. An error will be
returned if the wrong key is provided.

Response: standard.

#### /wallet/address [GET]

Function: Get a new address from the wallet generated by the primary seed. An
error will be returned if the wallet is locked.

Parameters: none

Response:
```
struct {
	address types.UnlockHash (string)
}
```
'address' is a wallet address that can receive siacoins or siafunds.

#### /wallet/addresses [GET]

Function: Fetch the list of addresses from the wallet.

Parameters: none

Response:
```
struct {
	addresses []types.UnlockHash (string)
}
```
'addresses' is an array of wallet addresses.

#### /wallet/backup [GET]

Function: Create a backup of the wallet settings file. Though this can easily
be done manually, the settings file is often in an unknown or difficult to find
location. The /wallet/backup call can spare users the trouble of needing to
find their wallet file.

Parameters:
```
destination string
```
'destination' is the location on disk where the file will be saved.

Response: standard

#### /wallet/init [POST]

Function: Initialize the wallet. After the wallet has been initialized once, it
does not need to be initialized again, and future calls to /wallet/init will
return an error. The encryption password is provided by the api call. If the
password is blank, then the password will be set to the same as the seed.

Parameters:
```
encryptionpassword string
dictionary string
```
'encryptionpassword' is the password that will be used to encrypt the wallet.
All subsequent calls should use this password. If left blank, the seed that
gets returned will also be the encryption password.

'dictionary' is the name of the dictionary that should be used when encoding
the seed. 'english' is the most common choice when picking a dictionary.

Response:
```
struct {
	primaryseed string
}
```
'primaryseed' is the dictionary encoded seed that is used to generate addresses
that the wallet is able to spend.

#### /wallet/seed [POST]

Function: Give the wallet a seed to track when looking for incoming
transactions. The wallet will be able to spend outputs related to addresses
created by the seed. The seed is added as an auxiliary seed, and does not
replace the primary seed. Only the primary seed will be used for generating new
addresses.

Parameters:
```
encryptionpassword string
dictionary         string
seed               string
```
'encryptionpassword' is the key that is used to encrypt the new seed when it is
saved to disk.

'dictionary' is the name of the dictionary that should be used when encoding
the seed. 'english' is the most common choice when picking a dictionary.

'seed' is the dictionary-encoded phrase that corresponds to the seed being
added to the wallet.

Response: standard

#### /wallet/seeds [GET]

Function: Return a list of seeds in use by the wallet. The primary seed is the
only seed that gets used to generate new addresses. This call is unavailable
when the wallet is locked.

Parameters:
```
dictionary string
```
'dictionary' is the name of the dictionary that should be used when encoding
the seed. 'english' is the most common choice when picking a dictionary.

Response:
```
struct {
	primaryseed        mnemonics.Phrase   (string)
	addressesremaining int
	allseeds           []mnemonics.Phrase ([]string)
}
```
'primaryseed' is the seed that is actively being used to generate new addresses
for the wallet.

'addressesremaining' is the number of addresses that remain in the primary seed
until exhaustion has been reached. Once exhaustion has been reached, new
addresses will continue to be generated but they will be more difficult to
recover in the event of a lost wallet file or encryption password.

'allseeds' is an array of all seeds that the wallet references when scanning the
blockchain for outputs. The wallet is able to spend any output generated by any
of the seeds, however only the primary seed is being used to generate new
addresses.

A seed is an encoded version of a 128 bit random seed. The output is 15 words
chosen from a small dictionary as indicated by the input. The most common
choice for the dictionary is going to be 'english'. The underlying seed is the
same no matter what dictionary is used for the encoding. The encoding also
contains a small checksum of the seed, to help catch simple mistakes when
copying. The library
[entropy-mnemonics](https://github.com/NebulousLabs/entropy-mnemonics) is used
when encoding.

#### /wallet/siacoins [POST]

Function: Send siacoins to an address. The outputs are arbitrarily selected
from addresses in the wallet.

Parameters:
```
amount      int
destination types.UnlockHash (string)
```
'amount' is the number of hastings being sent. A hasting is the smallest unit
in Sia. There are 10^24 hastings in a siacoin.

'destination' is the address that is receiving the coins.

Response:
```
struct {
	transactionids []types.TransactionID ([]string)
}
```
'transactionids' are the ids of the transactions that were created when sending
the coins. The last transaction contains the output headed to the
'destination'.

#### /wallet/siafunds [POST]

Function: Send siafunds to an address. The outputs are arbitrarily selected
from addresses in the wallet. Any siacoins available in the siafunds being sent
(as well as the siacoins available in any siafunds that end up in a refund
address) will become available to the wallet as siacoins after 144
confirmations. To access all of the siacoins in the siacoin claim balance, send
all of the siafunds to an address in your control (this will give you all the
siacoins, while still letting you control the siafunds).

Parameters:
```
amount      int
destination string
```
'amount' is the number of siafunds being sent.

'destination' is the address that is receiving the funds.

Response:
```
struct {
	transactionids []types.TransactionID ([]string)
}
```
'transactionids' are the ids of the transactions that were created when sending
the coins. The last transaction contains the output headed to the
'destination'.

#### /wallet/siagkey [POST]

Function: Load a key into the wallet that was generated by siag. Most siafunds
are currently in addresses created by siag.

Parameters:
```
encryptionpassword string
keyfiles           string
```
'encryptionpassword' is the key that is used to encrypt the siag key when it is
imported to the wallet.

'keyfiles' is a list of filepaths that point to the keyfiles that make up the
siag key. There should be at least one keyfile per required signature. The
filenames need to be commna separated (no spaces), which means filepaths that
contain a comma are not allowed.

#### /wallet/lock [POST]

Function: Locks the wallet, wiping all secret keys. After being locked, the
keys are encrypted. Queries for the seed, to send siafunds, and related queries
become unavailable. Queries concerning transaction history and balance are
still available.

Parameters: none

Response: standard.

#### /wallet/transaction/___:id___ [GET]

Function: Get the transaction associated with a specific transaction id.

Parameters:
```
id string
```
'id' is the ID of the transaction being requested.

Response:
```
struct {
	transaction modules.ProcessedTransaction
}
```

Processed transactions are transactions that have been processed by the wallet
and given more information, such as a confirmation height and a timestamp.
Processed transactions will always be returned in chronological order.

A processed transaction takes the following form:
```
struct modules.ProcessedTransaction {
	transaction           types.Transaction
	transactionid         types.TransactionID (string)
	confirmationheight    types.BlockHeight   (int)
	confirmationtimestamp types.Timestamp     (uint64)

	inputs  []modules.ProcessedInput
	outputs []modules.ProcessedOutput
}
```
'transaction' is a types.Transaction, and is defined in types/transactions.go

'transactionid' is the id of the transaction from which the wallet transaction
was derived.

'confirmationheight' is the height at which the transaction was confirmed. The
height will be set to 'uint64max' if the transaction has not been confirmed.

'confirmationtimestamp' is the time at which a transaction was confirmed. The
timestamp is an unsigned 64bit unix timestamp, and will be set to 'uint64max'
if the transaction is unconfirmed.

'inputs' is an array of processed inputs detailing the inputs to the
transaction. More information below.

'outputs' is an array of processed outputs detailing the outputs of
the transaction. Outputs related to file contracts are excluded.

A modules.ProcessedInput takes the following form:
```
struct modules.ProcessedInput {
	fundtype       types.Specifier  (string)
	walletaddress  bool
	relatedaddress types.UnlockHash (string)
	value          types.Currency   (string)
}
```

'fundtype' indicates what type of fund is represented by the input. Inputs can
be of type 'siacoin input', and 'siafund input'.

'walletaddress' indicates whether the address is owned by the wallet.
 
'relatedaddress' is the address that is affected. For inputs (outgoing money),
the related address is usually not important because the wallet arbitrarily
selects which addresses will fund a transaction. For outputs (incoming money),
the related address field can be used to determine who has sent money to the
wallet.

'value' indicates how much money has been moved in the input or output.

A modules.ProcessedOutput takes the following form:
```
struct modules.ProcessedOutput {
	fundtype       types.Specifier   (string)
	maturityheight types.BlockHeight (int)
	walletaddress  bool
	relatedaddress types.UnlockHash  (string)
	value          types.Currency    (string)
}
```

'fundtype' indicates what type of fund is represented by the output. Outputs
can be of type 'siacoin output', 'siafund output', and 'claim output'. Siacoin
outputs and claim outputs both relate to siacoins. Siafund outputs relate to
siafunds. Another output type, 'miner payout', points to siacoins that have been
spent on a miner payout. Because the destination of the miner payout is determined by
the block and not the transaction, the data 'maturityheight', 'walletaddress',
and 'relatedaddress' are left blank.

'maturityheight' indicates what height the output becomes available to be
spent. Siacoin outputs and siafund outputs mature immediately - their maturity
height will always be the confirmation height of the transaction. Claim outputs
cannot be spent until they have had 144 confirmations, thus the maturity height
of a claim output will always be 144 larger than the confirmation height of the
transaction.

'walletaddress' indicates whether the address is owned by the wallet.
 
'relatedaddress' is the address that is affected.

'value' indicates how much money has been moved in the input or output.

#### /wallet/transactions [GET]

Function: Return a list of transactions related to the wallet.

Parameters:
```
startheight types.BlockHeight (uint64)
endheight   types.BlockHeight (uint64)
```
'startheight' refers to the height of the block where transaction history
should begin.

'endheight' refers to the height of of the block where the transaction history
should end. If 'endheight' is greater than the current height, all transactions
up to and including the most recent block will be provided.

Response:
```
struct {
	confirmedtransactions   []modules.ProcessedTransaction
	unconfirmedtransactions []modules.ProcessedTransaction
}
```
'confirmedtransactions' lists all of the confirmed transactions appearing between
height 'startheight' and height 'endheight' (inclusive).

'unconfirmedtransactions' lists all of the unconfirmed transactions.

#### /wallet/transactions/___:addr___ [GET]

Function: Return all of the transaction related to a specific address.

Parameters:
```
addr types.UnlockHash
```
'addr' is the unlock hash (i.e. wallet address) whose transactions are being
requested.

Response:
```
struct {
	transactions []modules.ProcessedTransaction.
}
```
'transactions' is a list of processed transactions that relate to the supplied
address.  See the documentation for '/wallet/transaction' for more information.

#### /wallet/unlock [POST]

Function: Unlock the wallet. The wallet is capable of knowing whether the
correct password was provided.

Parameters:
```
encryptionpassword string
```
'encryptionpassword' is the password that gets used to decrypt the file. Most
frequently, the encryption password is the same as the primary wallet seed.

Response: standard
