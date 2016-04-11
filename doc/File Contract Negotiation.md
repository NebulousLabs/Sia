File Contract Negotiation
=========================

Securing data on Sia requires creating and revising file contracts in an
untrusted environment. Managing data on Sia happens through several protocols:

+ Settings Request - the host sends the renter its settings.

+ File Contract Creation - no data is uploaded during the inital creation of a
  file contract, but funds are allocated so that the file contract can be
  iteratively revised in the future.

+ File Contract Revision - an existing file contract is revised so that data
  can be added to an arbitrary place, or removed from an arbitrary place.

+ File Contract Renewal - an existing file contract is renewed, meaning that a
  new file contract with a different id is created, but that has the same data.
  New funds are added to this file contract, and it can now be modified
  separately from the previous contract.

+ Data Request - data is requested from the host for retrieval.

### Extra protocols will eventually be implemented, but not until after the
### v1.0 release.

# Storage Proof Request - the renter requests that the host perform an
# out-of-band storage proof.

# Metadata Request - the renter requests some metadata about the file contract
# from the host, namely the list of hashes that compose the file. This list of
# hashes is provided along with a cryptographic proof that the hashes are
# valid. The proof is only needed if only a subset of hashes are being sent.

A frequently seen construction is 'acceptance'. The renter or host may have the
opportunity to accept or reject a communication, which takes the form of a
string. The acceptance string is always the same, and any string that is not
the acceptance string is a rejection. The rejection string can include reasons
why the rejection occurred, but most not exceed 255 bytes. After a rejection,
the connection is always closed.

The protocols described below are numbered. The number indicates when the
communicator is switching. Each pair of numbers is a full round trip of
communications.

All communications attempt to support slow connections and Tor connections. Any
connection with a throughput below 100kbps may struggle to perform the uploads
and downloads, and any connection with a rountrip latency greater than 2
minutes may struggle to complete the protocols.

Settings Request
----------------

1. The renter makes an RPC to the host, opening a connection. The connection
   deadline should be at least 120 seconds.

2. The host sends the renter the most recent copy of its internal settings,
   signed by the host public key. The connection is then closed.

File Contract Creation
----------------------

1. The renter makes an RPC to the host, opening a connection. The connection
   deadline should be at least 360 seconds.

2. The host sends the renter the most recent copy of its settings, signed. If
   the host is not accepting new file contracts, the connection is closed.

# Witholding the signature is not strictly necessary, but if the signature is
# not withheld, the host has not yet committed to accepting the file contract,
# and so can appear to reject the file contract while still submitting it to the
# blockchain. The renter will not know until the file contract appears on the
# blockchain. If the renter signs at this point, the renter will also not be
# able to sign the whole transaction because the host must add collateral. This
# means that the renter will not even know the file contract id.
3. The renter sends a notice of acceptance or rejection. If the renter accepts,
   the renter then sends a funded file contract without a signature, followed
   by the public key that will be used to create the renter's portion of the
   UnlockConditions for the file contract.

# The host must always sign last, lest the renter trick the host into storing
# data for free. Only the new data is sent to the renter, both to make
# programming against the TransactionBuilder easier but also so that there's no
# risk to the renter that other fields (such as the file contract) have been
# changed.
4. The host sends an acceptance or rejection of the file contract. If the host
   accepts, the host will add collateral (and maybe miner fees) to the file
   contract, and will send the renter the inputs + outputs for the collateral,
   followed by any new parent transactions. The length of any of these may be
   zero.

# Only the transaction signatures are sent because the file contract is
# supposed to be finalized at this point.
5. The renter indicates acceptance or rejection of the file contract. If the
   renter accepts, the renter will sign the file contract and send the
   transaction signatures to the host.

6. The host may only reject the file contract in the event that the renter has
   sent invalid signatures, so the acceptance step is skipped. The host signs
   the file contract and sends the transaction signatures to the renter. The
   connection is closed.

File Contract Revision
----------------------

1. The renter makes an RPC to the host, opening a connection. The minimum
   deadline for the connection is 600 seconds. The renter then sends a file
   contract ID, indicting the file contract that is getting revised during the
   RPC.

2. The host will send an acceptance or rejection. A loop begins. The host sends
   the most recent revision of the host settings to the renter, and a copy of
   the most recent known revision transaction set. The transaction will be empty if
   there have been no completed revisions yet. The host is expected to always
   have the most recent revision, the renter may not have the most recent
   revision. The settings are sent after each iteration of the loop to enable
   high resolution dynamic pricing for the host, especially for bandwidth.

3. The renter may reject or accept the settings + revision. The renter will
   send a batch of modification actions. Batching allows the renter to send a
   lot of data in a single, one-way connection, improving throughput. The
   renter will send a number indicating how many modifications will be made in
   a batch, and then sends each modification in order.

   A single modification can either be an insert, a modify, or a delete. An
   insert is an index, indicating the index where the data is going to be
   inserted. '0' indicates that the data is inserted at the very beginning, '1'
   indicates that the data will be inserted between the first and second
   existing sectors, etc. The index is followed by the 4MB of data. A modify is
   an index indicating which sector is being modified, followed by an offset
   indicating which data within the sector is being modified. Finally, some
   data is provided indicating what the data in the sector should be replaced
   with starting from that offset. The offset + len of the data should not
   exceed the sector size of 4MB. A delete is an index indicating the index of
   the sector that is being deleted. Each operation within a batch is atomic,
   meaning if you are inserting 3 sectors at the front of the file contract,
   the indexes of each should be '0', '1', '2'.

   The renter sends a file contract revision which pays the host for all of the
   modifications and bandwidth consumed. The revision is not signed.

4. The host indicates either acceptance or rejection of the new revision.

5. The renter signs the revision and sends the signature to the host. The
   renter will then send an indication of whether another iteration of the loop
   is desired.

6. The host signs the revision and sends the siganture to the renter. Both
   parties submit the new revision to the transaction pool. The host sends an
   acceptance or rejection indicating whether another iteration is okay. If
   another iteration is to be performed, the connection deadline will be reset
   so that there are at least 600 seconds remaining.

File Contract Renewal
---------------------

1. The renter makes an RPC to the host, opening a connection. The minimum
   deadline for the connection is 600 seconds. The host needs extra time
   because a significant amount of metadata modifications may be necessary on
   the host's end, especially when renewing larger file contracts.

2. The host sends the most recent copy of the settings to the renter. If the
   host is not accepting new file contracts, the connection is closed.

3. The renter either accepts or rejects the settings. If accepted, the renter
   sends an unsigned file contract to the host, containing the same Merkle root
   as the previous file contract, and also containing a renewed payout with
   conditional payments to the host to cover the host storing the data for the
   extended duration.

4. The host will accept or reject the renewed file contract. If accepted, the
   host will add collateral (and miner fees if desired) and return the file
   contract unsigned to the renter.

5. The renter will accept or reject the host's additions. If accepting, the
   renter will sign the file contract and return it to the host.

6. The host will accept or reject the renter's signature on the file contract.
   If accepting, the host will add its signature and return the complete, fully
   signed file contract to the renter. The renter and the host may each now
   submit the file contract to their transaction pools.

Data Request
------------

1. The renter makes an RPC to the host, opening a connection. The connection
   deadline is at least 600 seconds.

2. A loop begins, which will allow the renter to download multiple batches of
   data from the same connection. The host will send the host settings, and the
   most recent file contract revision transaction. If there is no revision yet,
   the host will send a blank transaction. The host is expected to always have
   the most recent revision (the host signs last), the renter may not have the
   most recent revision.

3. The renter will accept or reject the host's settings. If accepting, the
   renter will send a batch of download requests, which takes the form of a
   batch size followed by each of the requests in order. This request is
   followed by a file contract revision, which pays the host for the download
   bandwidth that is about to be consumed. The revision will be signed. The
   renter will also send a variable indicating whether another iteration is
   desired.

   A download request can either be a full sector or a partial sector. A full
   sector request will be followed by the hash of the sector. A partial sector
   request will be followed by the hash of the sector that is being partially
   downloaded, along with an offset and a length indicating which portion of
   the sector is being downloaded.

4. The host will either accept or reject the revision. If accepted, the host
   will upload all of the data. The host will indicate whether another
   iteration is okay. If another iteration is acceptable, the deadline will be
   reset to a minimum of 600 seconds. The host is expected to accept at least
   1200 seconds of iterations.
