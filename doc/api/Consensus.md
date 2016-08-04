Consensus API
=============

This document contains detailed descriptions of the consensus's API routes. For
an overview of the consensus' API routes, see
[API.md#consensus](/doc/API.md#consensus).  For an overview of all API routes,
see [API.md](/doc/API.md)

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Overview
--------

The consensus set manages everything related to consensus and keeps the
blockchain in sync with the rest of the network. The consensus set's API
endpoint returns information about the state of the blockchain.

Index
-----

| Route                        | HTTP verb |
| ---------------------------- | --------- |
| [/consensus](#consensus-get) | GET       |

#### /consensus [GET]

returns information about the consensus set, such as the current block height.

###### JSON Response
```javascript
{
  // True if the consensus set is synced with the network, i.e. it has downloaded the entire blockchain.
  "synced": true,

  // Number of blocks preceding the current block.
  "height": 62248,

  // Hash of the current block.
  "currentblock": "00000000000008a84884ba827bdc868a17ba9c14011de33ff763bd95779a9cf1",

  // An immediate child block of this block must have a hash less than this
  // target for it to be valid.
  "target": [0,0,0,0,0,0,11,48,125,79,116,89,136,74,42,27,5,14,10,31,23,53,226,238,202,219,5,204,38,32,59,165]
}
```
