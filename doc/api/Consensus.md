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
    "height": 20032,
    "id": "00000000000033b9eb57fa63a51adeea857e70f6415ebbfe5df2a01f0d0477f4",
    "minerpayouts": [
        {
            "unlockhash": "c199cd180e19ef7597bcf4beecdd4f211e121d085e24432959c42bdf9030e32b9583e1c2727c",
            "value": "279978000000000000000000000000"
        }
    ],
    "nonce": [4,12,219,7,0,0,0,0],
    "parentid": "0000000000009615e8db750eb1226aa5e629bfa7badbfe0b79607ec8b918a44c",
    "timestamp": 1444516982,
    "transactions": [
        {
	    // ...
        },
        {
            "arbitrarydata": [],
            "filecontractrevisions": [],
            "filecontracts": [],
            "id": "3c98ec79b990461f353c22bb06bcfb10e702f529ad7d27a43c4448273553d90a",
            "minerfees": [],
            "siacoininputs": [
                {
                    "parentid": "24cbeb9df7eb2d81d0025168fc94bd179909d834f49576e65b51feceaf957a64",
                    "unlockconditions": {
                        "publickeys": [
                            {
                                "algorithm": "ed25519",
                                "key": "QET8w7WRbGfcnnpKd1nuQfE3DuNUUq9plyoxwQYDK4U="
                            }
                        ],
                        "signaturesrequired": 1,
                        "timelock": 0
                    }
                }
            ],
            "siacoinoutputs": [
                {
                    "id": "1f9da81e23522f79590ac67ac0b668828c52b341cbf04df4959bb7040c072f29",
                    "unlockhash": "d54f500f6c1774d518538dbe87114fe6f7e6c76b5bc8373a890b12ce4b8909a336106a4cd6db",
                    "value": "1010000000000000000000000000"
                },
                {
                    "id": "14978a4c54f5ebd910ea41537de014f8423574c13d132e8713fab5af09ec08ca",
                    "unlockhash": "48a56b19bd0be4f24190640acbd0bed9669ea9c18823da2645ec1ad9652f10b06c5d4210f971",
                    "value": "5780000000000000000000000000"
                }
            ],
            "siafundinputs": [],
            "siafundoutputs": [],
            "storageproofs": [],
            "transactionsignatures": [
                {
                    "coveredfields": {
                        "arbitrarydata": [],
                        "filecontractrevisions": [],
                        "filecontracts": [],
                        "minerfees": [],
                        "siacoininputs": [],
                        "siacoinoutputs": [],
                        "siafundinputs": [],
                        "siafundoutputs": [],
                        "storageproofs": [],
                        "transactionsignatures": [],
                        "wholetransaction": true
                    },
                    "parentid": "24cbeb9df7eb2d81d0025168fc94bd179909d834f49576e65b51feceaf957a64",
                    "publickeyindex": 0,
                    "signature": "pByLGMlvezIZWVZmHQs/ynGETETNbxcOY/kr6uivYgqZqCcKTJ0JkWhcFaKJU+3DEA7JAloLRNZe3PTklD3tCQ==",
                    "timelock": 0
                }
            ]
        },
        {
	    // ...
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
