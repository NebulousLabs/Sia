Transaction Pool API
=========

This document contains detailed descriptions of the transaction pool's API
routes. For an overview of the transaction pool's API routes, see
[API.md#transactionpool](/doc/API.md#transactionpool).  For an overview of all
API routes, see [API.md](/doc/API.md)

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Overview
--------

The transaction pool provides endpoints for getting transactions currently in
the transaction pool and submitting transactions to the transaction pool.

Index
-----

| Route                           | HTTP verb |
| ------------------------------- | --------- |
| [/tpool/raw/:id](#tpoolraw-get) | GET       |
| [/tpool/raw](#tpoolraw-post)    | POST      |

#### /tpool/raw/:id [GET]

returns the ID for the requested transaction and its raw encoded parents and transaction data.

###### JSON Response [(with comments)](/doc/api/Transactionpool.md#json-response)
```javascript
{
	// id of the transaction
	"id": "1234567890abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	// raw transaction data
	"transaction": "AAAAAQID",
	"parents": "TWFuIGlzIGRpc3Rpbmd1aXNoZWQsIG5vdCBvbmx5IGJ5IGhpcyByZWFzb24sIGJ1dCBieSB0aGlz",
}
```

#### /tpool/raw [POST]

submits a raw transaction to the transaction pool, broadcasting it to the transaction pool's peers.

###### Query String Parameters [(with comments)](/doc/api/Transactionpool.md#query-string-parameters)

```
parents string // raw encoded transaction parents
transaction string // raw encoded transaction
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

