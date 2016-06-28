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

| Request                                     | HTTP Verb | Examples                      |
| ------------------------------------------- | --------- | ----------------------------- |
| [/hostdb/active](#hostdbactive-get-example) | GET       | [Active hosts](#active-hosts) |
| [/hostdb/all](#hostdball-get-example)       | GET       | [All hosts](#all-hosts)       |

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

Examples
--------

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
    }
  ]
}
```
