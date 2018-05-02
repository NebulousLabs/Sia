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

| Route                                                                       | HTTP verb |
| --------------------------------------------------------------------------- | --------- |
| [/consensus](#consensus-get)                                                | GET       |
| [/consensus/blocks](#consensusblocks-get)                                   | GET       |
| [/consensus/validate/transactionset](#consensusvalidatetransactionset-post) | POST      |

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
  "target": [0,0,0,0,0,0,11,48,125,79,116,89,136,74,42,27,5,14,10,31,23,53,226,238,202,219,5,204,38,32,59,165],

  // The difficulty of the current block target.
  "difficulty": "1234" // arbitrary-precision integer
}
```

#### /consensus/blocks [GET]

Returns the block for a given id or height.

###### Query String Parameters
One of the following parameters can be specified.
```
// BlockID of the requested block.
id 

// BlockHeight of the requested block.
height

```

###### Response
The JSON formatted block or a standard error response.
```
{
    "height": 1,
    "id": "0000000018492dfe2a1b2da6ca3534a757796573f84f0eb0eb5f88d75cd10f9f",
    "minerpayouts": [
        {
            "unlockhash": "68a0607b15f21ce643bb47075e539b9e938d7a8e0870951a3c872a2d89ae05194ee3a0f18680",
            "value": "299999000000000000000000000000"
        }
    ],
    "nonce": [253,253,0,0,0,0,0,100],
    "parentid": "25f6e3b9295a61f69fcb956aca9f0076234ecf2e02d399db5448b6e22f26e81c",
    "timestamp": 1433626546,
    "transactions": [
        {
            "arbitrarydata": [
                "Tm9uU2lhQ9ikb2Lc6jDRfLhhjPflTQ=="
            ],
            "filecontractrevisions": [],
            "filecontracts": [],
            "minerfees": [],
            "siacoininputs": [],
            "siacoinoutputs": [],
            "siafundinputs": [],
            "siafundoutputs": [],
            "storageproofs": [],
            "transactionsignatures": []
        }
    ]
}
```

#### /consensus/validate/transactionset [POST]

validates a set of transactions using the current utxo set.

###### Request Body Bytes

Since transactions may be large, the transaction set is supplied in the POST
body, encoded in JSON format.

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).
