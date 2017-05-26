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
	"id": "124302d30a219d52f368ecd94bae1bfb922a3e45b6c32dd7fb5891b863808788",

	// raw transaction data
	"transaction": "AQAAAAAAAADBM1ca/FyURfizmSukoUQ2S0GwXMit1iNSeYgrnhXOPAAAAAAAAAAAAQAAAAAAAABlZDI1NTE5AAAAAAAAAAAAIAAAAAAAAACdfzoaJ1MBY7L0fwm7O+BoQlFkkbcab5YtULa6B9aecgEAAAAAAAAAAQAAAAAAAAAMAAAAAAAAAAM7Ljyf0IA86AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAACgAAAAAAAACe0ZTbGbI4wAAAAAAAAAAAAAABAAAAAAAAAMEzVxr8XJRF+LOZK6ShRDZLQbBcyK3WI1J5iCueFc48AAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAA+z4P1wc98IqKxykTSJxiVT+BVbWezIBnIBO1gRRlLq2x/A+jIc6G7/BA5YNJRbdnqPHrzsZvkCv4TKYd/XzwBA==",
	"parents": "AQAAAAAAAAABAAAAAAAAAJYYmFUdXXfLQ2p6EpF+tcqM9M4Pw5SLSFHdYwjMDFCjAAAAAAAAAAABAAAAAAAAAGVkMjU1MTkAAAAAAAAAAAAgAAAAAAAAAAHONvdzzjHfHBx6psAN8Z1rEVgqKPZ+K6Bsqp3FbrfjAQAAAAAAAAACAAAAAAAAAAwAAAAAAAAAAzvNDjSrme8gwAAA4w8ODnW8DxbOV/JribivvTtjJ4iHVOug0SXJc31BdSINAAAAAAAAAAPGHY4699vggx5AAAC2qBhm5vwPaBsmwAVPho/1Pd8ecce/+BGv4UimnEPzPQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAACWGJhVHV13y0NqehKRfrXKjPTOD8OUi0hR3WMIzAxQowAAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAAAAAAAAABnt64wN1qxym/CfiMgOx5fg/imVIEhY+4IiiM7gwvSx8qtqKniOx50ekrGv8B+gTKDXpmm2iJibWTI9QLZHWAY=",
}
```

#### /tpool/raw [POST]

submits a raw transaction to the transaction pool, broadcasting it to the transaction pool's peers.

###### Query String Parameters [(with comments)](/doc/api/Transactionpool.md#query-string-parameters)

```
parents string     // raw encoded transaction parents
transaction string // raw encoded transaction
```

###### Response
standard success or error response. See
[#standard-responses](#standard-responses).

