Gateway API
===========

This document contains detailed descriptions of the gateway's API routes. For
an overview of the gateway's API routes, see
[API.md#gateway](/doc/API.md#gateway).  For an overview of all API routes, see
[API.md](/doc/API.md)

There may be functional API calls which are not documented. These are not
guaranteed to be supported beyond the current release, and should not be used
in production.

Overview
--------

The gateway maintains a peer to peer connection to the network and provides a
method for calling RPCs on connected peers. The gateway's API endpoints expose
methods for viewing the connected peers, manually connecting to peers, and
manually disconnecting from peers. The gateway may connect or disconnect from
peers on its own.

Index
-----

| Route | HTTP verb |
| ----- | --------- |
| [/gateway](#gateway-get) | GET |
| [/gateway/connect/{netaddress}](#gatewayconnectnetaddress-post) | POST |
| [/gateway/disconnect/{netaddress}](#gatewaydisconnectnetaddress-post) | POST |

Examples
--------

* [gateway info](#gateway-info-example)
* [connecting to a peer](#connect-example)
* [disconnecting from a peer](#disconnect-example)

-------------------------------------------------------------------------------

#### /gateway [GET]

`/gateway [GET]` returns information about the gateway, including the list of
connected peers.

###### JSON Response
```javascript
{
    // netaddress is the network address of the gateway as seen by the rest of
    // the network. The address consists of the external IP address and the
    // port Sia is listening on. It represents a `modules.NetAddress`.
    "netaddress": String,

    // peers is an array of peers the gateway is connected to. It represents
    // an array of `modules.Peer`s.
    "peers":      []{
        // netaddress is the address of the peer. It represents a
        // `modules.NetAddress`.
        "netaddress": String,

        // version is the version number of the peer.
        "version":    String,

        // inbound is true when the peer initiated the connection. This field
        // is exposed as outbound peers are generally trusted more than inbound
        // peers, as inbound peers are easily manipulated by an adversary.
        "inbound":    Boolean
    }
}
```

###### [Example](#gateway-info-example)

#### /gateway/connect/{netaddress} [POST]

`/gateway/connect/{netaddress} [POST]` connects the gateway to a peer. The peer
is added to the node list if it is not already present. The node list is the
list of all nodes the gateway knows about, but is not necessarily connected to.

###### Path Parameters
```
// netaddress is the address of the peer to connect to. It should be a
// reachable ip address and port number, of the form 'IP:port'. IPV6 addresses
// must be enclosed in square brackets.
//
// Example IPV4 address: 123.456.789.0:123
// Example IPV6 address: [123::456]:789
{netaddress}
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

###### [Example](#connect-example)

#### /gateway/disconnect/{netaddress} [POST]

`/gateway/disconnect/{netaddress} [POST]` disconnects the gateway from a peer.
The peer remains in the node list. Disconnecting from a peer does not prevent
the gateway from automatically connecting to the peer in the future.

###### Path Parameters
```
// netaddress is the address of the peer to connect to. It should be a
// reachable ip address and port number, of the form 'IP:port'. IPV6 addresses
// must be enclosed in square brackets.
//
// Example IPV4 address: 123.456.789.0:123
// Example IPV6 address: [123::456]:789
{netaddress}
```

###### Response
standard success or error response. See
[API.md#standard-responses](/doc/API.md#standard-responses).

###### [Example](#disconnect-example)

#### gateway info example

Request:
```
/gateway
```

Expected Response Code: 200

Example JSON Response:
```json
{
    "netaddress":"333.333.333.333:9981",
    "peers":[
        {
            "netaddress":"222.222.222.222:9981",
            "version":"1.0.0",
            "inbound":false
        },
        {
            "netaddress":"111.111.111.111:9981",
            "version":"0.6.0",
            "inbound":true
        }
    ]
}
```

#### connect example

Request:
```
/gateway/connect/123.456.789.0:123
```

Expected Response Code: 204

#### disconnect example

Request:
```
/gateway/disconnect/123.456.789.0:123
```

Expected Response Code: 204
