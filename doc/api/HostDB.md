Host DB API
===========

This document contains detailed descriptions of the hostdb's API routes. For an
overview of the hostdb's API routes, see [API.md#host-db](/doc/API.md#host-db).
For an overview of all API routes, see [API.md](/doc/API.md)

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Overview
--------

The hostdb maintains a database of all hosts known to the network. The database
identifies hosts by their public key and keeps track of metrics such as price.

Index
-----

| Request                                                 | HTTP Verb | Examples                      |
| ------------------------------------------------------- | --------- | ----------------------------- |
| [/hostdb](#hostdb-get-example)                          | GET       | [HostDB Get](#hostdb-get)     |
| [/hostdb/active](#hostdbactive-get-example)             | GET       | [Active hosts](#active-hosts) |
| [/hostdb/all](#hostdball-get-example)                   | GET       | [All hosts](#all-hosts)       |
| [/hostdb/hosts/___:pubkey___](#hostdbhosts-get-example) | GET       | [Hosts](#hosts)               |

#### /hostdb [GET] [(example)](#hostdb-get)

shows some general information about the state of the hostdb.

###### JSON Response 

Either the following JSON struct or an error response. See [#standard-responses](#standard-responses).

```javascript
{
    "initialscancomplete": false // indicates if all known hosts have been scanned at least once.
}
```

#### /hostdb/active [GET] [(example)](#active-hosts)

lists all of the active hosts known to the renter, sorted by preference.

###### Query String Parameters
```
// Number of hosts to return. The actual number of hosts returned may be less
// if there are insufficient active hosts. Optional, the default is all active
// hosts.
numhosts
```

###### JSON Response
```javascript
{
  "hosts": [
    {
      // true if the host is accepting new contracts.
      "acceptingcontracts": true,

      // Maximum number of bytes that the host will allow to be requested by a
      // single download request.
      "maxdownloadbatchsize": 17825792,

      // Maximum duration in blocks that a host will allow for a file contract.
      // The host commits to keeping files for the full duration under the
      // threat of facing a large penalty for losing or dropping data before
      // the duration is complete. The storage proof window of an incoming file
      // contract must end before the current height + maxduration.
      //
      // There is a block approximately every 10 minutes.
      // e.g. 1 day = 144 blocks
      "maxduration": 25920,

      // Maximum size in bytes of a single batch of file contract
      // revisions. Larger batch sizes allow for higher throughput as there is
      // significant communication overhead associated with performing a batch
      // upload.
      "maxrevisebatchsize": 17825792,

      // Remote address of the host. It can be an IPv4, IPv6, or hostname,
      // along with the port. IPv6 addresses are enclosed in square brackets.
      "netaddress": "123.456.789.0:9982",

      // Unused storage capacity the host claims it has, in bytes.
      "remainingstorage": 35000000000,

      // Smallest amount of data in bytes that can be uploaded or downloaded to
      // or from the host.
      "sectorsize": 4194304,

      // Total amount of storage capacity the host claims it has, in bytes.
      "totalstorage": 35000000000,

      // Address at which the host can be paid when forming file contracts.
      "unlockhash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",

      // A storage proof window is the number of blocks that the host has to
      // get a storage proof onto the blockchain. The window size is the
      // minimum size of window that the host will accept in a file contract.
      "windowsize": 144,

      // Public key used to identify and verify hosts.
      "publickey": {
        // Algorithm used for signing and verification. Typically "ed25519".
        "algorithm": "ed25519",

        // Key used to verify signed host messages.
        "key": "RW50cm9weSBpc24ndCB3aGF0IGl0IHVzZWQgdG8gYmU="
      }
    }
  ]
}
```

#### /hostdb/all [GET] [(example)](#all-hosts)

lists all of the hosts known to the renter. Hosts are not guaranteed to be in
any particular order, and the order may change in subsequent calls.

###### JSON Response
```javascript
{
  "hosts": [
    {
      // true if the host is accepting new contracts.
      "acceptingcontracts": true,

      // Maximum number of bytes that the host will allow to be requested by a
      // single download request.
      "maxdownloadbatchsize": 17825792,

      // Maximum duration in blocks that a host will allow for a file contract.
      // The host commits to keeping files for the full duration under the
      // threat of facing a large penalty for losing or dropping data before
      // the duration is complete. The storage proof window of an incoming file
      // contract must end before the current height + maxduration.
      //
      // There is a block approximately every 10 minutes.
      // e.g. 1 day = 144 blocks
      "maxduration": 25920,

      // Maximum size in bytes of a single batch of file contract
      // revisions. Larger batch sizes allow for higher throughput as there is
      // significant communication overhead associated with performing a batch
      // upload.
      "maxrevisebatchsize": 17825792,

      // Remote address of the host. It can be an IPv4, IPv6, or hostname,
      // along with the port. IPv6 addresses are enclosed in square brackets.
      "netaddress": "123.456.789.0:9982",

      // Unused storage capacity the host claims it has, in bytes.
      "remainingstorage": 35000000000,

      // Smallest amount of data in bytes that can be uploaded or downloaded to
      // or from the host.
      "sectorsize": 4194304,

      // Total amount of storage capacity the host claims it has, in bytes.
      "totalstorage": 35000000000,

      // Address at which the host can be paid when forming file contracts.
      "unlockhash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",

      // A storage proof window is the number of blocks that the host has to
      // get a storage proof onto the blockchain. The window size is the
      // minimum size of window that the host will accept in a file contract.
      "windowsize": 144,

      // Public key used to identify and verify hosts.
      "publickey": {
        // Algorithm used for signing and verification. Typically "ed25519".
        "algorithm": "ed25519",

        // Key used to verify signed host messages.
        "key": "RW50cm9weSBpc24ndCB3aGF0IGl0IHVzZWQgdG8gYmU="
      }
    }
  ]
}
```

#### /hostdb/hosts/___:pubkey___ [GET] [(example)](#hosts)

fetches detailed information about a particular host, including metrics
regarding the score of the host within the database. It should be noted that
each renter uses different metrics for selecting hosts, and that a good score on
in one hostdb does not mean that the host will be successful on the network
overall.

###### Path Parameters
```
// The public key of the host. Each public key identifies a single host.
//
// Example Pubkey: ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef

:pubkey
```

###### JSON Response
```javascript
{
  "entry": {
    // true if the host is accepting new contracts.
    "acceptingcontracts": true,

    // Maximum number of bytes that the host will allow to be requested by a
    // single download request.
    "maxdownloadbatchsize": 17825792,

    // Maximum duration in blocks that a host will allow for a file contract.
    // The host commits to keeping files for the full duration under the
    // threat of facing a large penalty for losing or dropping data before
    // the duration is complete. The storage proof window of an incoming file
    // contract must end before the current height + maxduration.
    //
    // There is a block approximately every 10 minutes.
    // e.g. 1 day = 144 blocks
    "maxduration": 25920,

    // Maximum size in bytes of a single batch of file contract
    // revisions. Larger batch sizes allow for higher throughput as there is
    // significant communication overhead associated with performing a batch
    // upload.
    "maxrevisebatchsize": 17825792,

    // Remote address of the host. It can be an IPv4, IPv6, or hostname,
    // along with the port. IPv6 addresses are enclosed in square brackets.
    "netaddress": "123.456.789.0:9982",

    // Unused storage capacity the host claims it has, in bytes.
    "remainingstorage": 35000000000,

    // Smallest amount of data in bytes that can be uploaded or downloaded to
    // or from the host.
    "sectorsize": 4194304,

    // Total amount of storage capacity the host claims it has, in bytes.
    "totalstorage": 35000000000,

    // Address at which the host can be paid when forming file contracts.
    "unlockhash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",

    // A storage proof window is the number of blocks that the host has to
    // get a storage proof onto the blockchain. The window size is the
    // minimum size of window that the host will accept in a file contract.
    "windowsize": 144,

    // Public key used to identify and verify hosts.
    "publickey": {
      // Algorithm used for signing and verification. Typically "ed25519".
      "algorithm": "ed25519",

      // Key used to verify signed host messages.
      "key": "RW50cm9weSBpc24ndCB3aGF0IGl0IHVzZWQgdG8gYmU="
    },

    // The string representation of the full public key, used when calling
    // /hostdb/hosts.
    "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
  },

  // A set of scores as determined by the renter. Generally, the host's final
  // final score is all of the values multiplied together. Modified renters may
  // have additional criteria that they use to judge a host, or may ignore
  // certin criteia. In general, these fields should only be used as a loose
  // guide for the score of a host, as every renter sees the world differently
  // and uses different metrics to evaluate hosts.
  "scorebreakdown": {
	// The overall score for the host. Scores are entriely relative, and are
	// consistent only within the current hostdb. Between different machines,
	// different configurations, and different versions the absolute scores for
	// a given host can be off by many orders of magnitude. When displaying to a
	// human, some form of normalization with respect to the other hosts (for
	// example, divide all scores by the median score of the hosts) is
	// recommended.
	"score":                      123456,

    // The multiplier that gets applied to the host based on how long it has
    // been a host. Older hosts typically have a lower penalty.
    "ageadjustment":              0.1234,

    // The multiplier that gets applied to the host based on how much
    // proof-of-burn the host has performed. More burn causes a linear increase
    // in score.
    "burnadjustment":             23.456,

    // The multiplier that gets applied to a host based on how much collateral
    // the host is offering. More collateral is typically better, though above
    // a point it can be detrimental.
    "collateraladjustment":       23.456,

    // The multipler that gets applied to a host based on previous interactions
    // with the host. A high ratio of successful interactions will improve this
    // hosts score, and a high ratio of failed interactions will hurt this
    // hosts score. This adjustment helps account for hosts that are on
    // unstable connections, don't keep their wallets unlocked, ran out of
    // funds, etc.
    "interactionadjustment":      0.1234,

    // The multiplier that gets applied to a host based on the host's price.
    // Lower prices are almost always better. Below a certain, very low price,
    // there is no advantage.
    "priceadjustment":            0.1234,

    // The multiplier that gets applied to a host based on how much storage is
    // remaining for the host. More storage remaining is better, to a point.
    "storageremainingadjustment": 0.1234,

    // The multiplier that gets applied to a host based on the uptime percentage
    // of the host. The penalty increases extremely quickly as uptime drops
    // below 90%.
    "uptimeadjustment":           0.1234,

    // The multiplier that gets applied to a host based on the version of Sia
    // that they are running. Versions get penalties if there are known bugs,
    // scaling limitations, performance limitations, etc. Generally, the most
    // recent version is always the one with the highest score.
    "versionadjustment":          0.1234
  }
}
```

Examples
--------

#### HostDB Get

###### Request
```
/hostdb
```

###### Expected Response Code
```
200 OK
```

###### Example JSON Response
```javascript
{
    "initialscancomplete": false
}
```

#### Active hosts

###### Request
```
/hostdb/active?numhosts=2
```

###### Expected Response Code
```
200 OK
```

###### Example JSON Response
```javascript
{
  "hosts": [
    {
      "acceptingcontracts": true,
      "maxdownloadbatchsize": 17825792,
      "maxduration": 25920,
      "maxrevisebatchsize": 17825792,
      "netaddress": "123.456.789.0:9982",
      "remainingstorage": 35000000000,
      "sectorsize": 4194304,
      "totalstorage": 35000000000,
      "unlockhash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
      "windowsize": 144,
      "publickey": {
        "algorithm": "ed25519",
        "key": "RW50cm9weSBpc24ndCB3aGF0IGl0IHVzZWQgdG8gYmU="
      }
      "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    },
    {
      "acceptingcontracts": true,
      "maxdownloadbatchsize": 17825792,
      "maxduration": 25920,
      "maxrevisebatchsize": 17825792,
      "netaddress": "123.456.789.1:9982",
      "remainingstorage": 314,
      "sectorsize": 4194304,
      "totalstorage": 314159265359,
      "unlockhash": "ba9876543210fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
      "windowsize": 144,
      "publickey": {
        "algorithm": "ed25519",
        "key": "WWVzIEJydWNlIFNjaG5laWVyIGNhbiByZWFkIHRoaXM="
      }
      "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    }
  ]
}
```

#### All hosts

###### Request
```
/hostdb/all
```

###### Expected Response Code
```
200 OK
```

###### Example JSON Response
```javascript
{
  "hosts": [
    {
      "acceptingcontracts": false,
      "maxdownloadbatchsize": 17825792,
      "maxduration": 25920,
      "maxrevisebatchsize": 17825792,
      "netaddress": "123.456.789.2:9982",
      "remainingstorage": 314,
      "sectorsize": 4194304,
      "totalstorage": 314159265359,
      "unlockhash": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
      "windowsize": 144,
      "publickey": {
        "algorithm": "ed25519",
        "key": "SSByYW4gb3V0IG9mIDMyIGNoYXIgbG9uZyBqb2tlcy4="
      }
      "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    },
    {
      "acceptingcontracts": true,
      "maxdownloadbatchsize": 17825792,
      "maxduration": 25920,
      "maxrevisebatchsize": 17825792,
      "netaddress": "123.456.789.0:9982",
      "remainingstorage": 35000000000,
      "sectorsize": 4194304,
      "totalstorage": 35000000000,
      "unlockhash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
      "windowsize": 144,
      "publickey": {
        "algorithm": "ed25519",
        "key": "RW50cm9weSBpc24ndCB3aGF0IGl0IHVzZWQgdG8gYmU="
      }
      "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    },
    {
      "acceptingcontracts": true,
      "maxdownloadbatchsize": 17825792,
      "maxduration": 25920,
      "maxrevisebatchsize": 17825792,
      "netaddress": "123.456.789.1:9982",
      "remainingstorage": 314,
      "sectorsize": 4194304,
      "totalstorage": 314159265359,
      "unlockhash": "ba9876543210fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
      "windowsize": 144,
      "publickey": {
        "algorithm": "ed25519",
        "key": "WWVzIEJydWNlIFNjaG5laWVyIGNhbiByZWFkIHRoaXM="
      }
      "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    }
  ]
}
```

#### Hosts

###### Request
```
/hostdb/hosts/ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef
```

###### Expected Response Code
```
200 OK
```

###### Example JSON Response
```javascript
{
  "entry": {
    "acceptingcontracts": false,
    "maxdownloadbatchsize": 17825792,
    "maxduration": 25920,
    "maxrevisebatchsize": 17825792,
    "netaddress": "123.456.789.2:9982",
    "remainingstorage": 314,
    "sectorsize": 4194304,
    "totalstorage": 314159265359,
    "unlockhash": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
    "windowsize": 144,
    "publickey": {
      "algorithm": "ed25519",
      "key": "SSByYW4gb3V0IG9mIDMyIGNoYXIgbG9uZyBqb2tlcy4="
    }
    "publickeystring": "ed25519:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
  },
  "scorebreakdown": {
    "ageadjustment": 0.1234,
    "burnadjustment": 0.1234,
    "collateraladjustment": 23.456,
    "priceadjustment": 0.1234,
    "storageremainingadjustment": 0.1234,
    "uptimeadjustment": 0.1234,
    "versionadjustment": 0.1234,
  }
}
```
