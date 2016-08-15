Miner API
=========

This document contains detailed descriptions of the miner's API routes. For an
overview of the miner's API routes, see [API.md#miner](/doc/API.md#miner).  For
an overview of all API routes, see [API.md](/doc/API.md)

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Overview
--------

The miner provides endpoints for getting headers for work and submitting solved
headers to the network. The miner also provides endpoints for controlling a
basic CPU mining implementation.

Index
-----

| Route                              | HTTP verb |
| ---------------------------------- | --------- |
| [/miner](#miner-get)               | GET       |
| [/miner/start](#minerstart-get)    | GET       |
| [/miner/stop](#minerstop-get)      | GET       |
| [/miner/header](#minerheader-get)  | GET       |
| [/miner/header](#minerheader-post) | POST      |

#### /miner [GET]

returns the status of the miner.

###### JSON Response
```javascript
{
  // Number of mined blocks. This value is remembered after restarting.
  "blocksmined": 9001,

  // How fast the cpu is hashing, in hashes per second.
  "cpuhashrate": 1337,

  // true if the cpu miner is active.
  "cpumining": false,

  // Number of mined blocks that are stale, indicating that they are not
  // included in the current longest chain, likely because some other block at
  // the same height had its chain extended first.
  "staleblocksmined": 0,
}
```

#### /miner/start [GET]

starts a single threaded cpu miner. Does nothing if the cpu miner is already
running.

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /miner/stop [GET]

stops the cpu miner. Does nothing if the cpu miner is not running.

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

#### /miner/header [GET]

provides a block header that is ready to be grinded on for work.

###### Byte Response

For efficiency the header for work is returned as a raw byte encoding of the
header, rather than encoded to JSON.

The response bytes contain a target followed by a block header
followed by a block. The target is the first 32 bytes. The block header is the
following 80 bytes, and the nonce is bytes 32-39 (inclusive) of the header
(bytes 64-71 of the whole array).

Layout:

0-31: target

32-111: header

```
xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx (returned bytes)
tttttttttttttttttttttttttttttttt (target)
                                hhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhhh (header)
                                                                nnnnnnnn (nonce)
```

#### /miner/header [POST]

submits a header that has passed the POW.

###### Request Body Bytes

For efficiency headers are submitted as raw byte encodings of the header in the
body of the request, rather than as a query string parameter or path parameter.
The request body should contain only the 80 bytes of the encoded header. The
encoding is the same encoding used in `/miner/header [GET]` endpoint. Refer to
[#byte-response](#byte-response) for a detailed description of the byte
encoding.
