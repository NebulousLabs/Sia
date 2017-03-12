File Contract Negotiation
=========================

Securing data on Sia requires creating and revising file contracts in an
untrusted environment. Managing data on Sia happens through several protocols:

+ Settings Request - the host sends the renter its settings.

+ Revision Request - the renter will send the host a file contract id, and the
  host will send the most recent file contract revision that it knows of for
  that file contract, with the signatures. A challenge and response is also
  performed to verify that the renter is able to create the signatures to
  modify the file contract revision.

+ File Contract Creation - no data is uploaded during the initial creation of a
  file contract, but funds are allocated so that the file contract can be
  iteratively revised in the future.

+ File Contract Revision - an existing file contract is revised so that data
  can be added to an arbitrary place, or removed from an arbitrary place.

+ File Contract Renewal - an existing file contract is renewed, meaning that a
  new file contract with a different id is created, but that has the same data.
  New funds are added to this file contract, and it can now be modified
  separately from the previous contract.

+ Data Request - data is requested from the host by hash.

+ (planned for later) Storage Proof Request - the renter requests that the host
  perform an out-of-band storage proof.

+ (planned for later) Metadata Request - the renter requests some metadata
  about the file contract from the host, namely the list of hashes that compose
  the file. This list of hashes is provided along with a cryptographic proof
  that the hashes are valid. The proof is only needed if only a subset of
  hashes are being sent.

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

The host signs the settings request to prove that the connection has opened to
the right party. Hosts announce on the blockchain and perform burn, therefore
identity is important.

1. The renter makes an RPC to the host, opening a connection. The connection
   deadline should be at least 120 seconds.

2. The host sends the renter the most recent copy of its external settings,
   signed by the host public key. The connection is then closed.

Revision Request
----------------

The renter requests a recent revision from the host. Often, this request
precedes modifications. A file contract can only be open for revision with one
party at a time. To prevent DoS attacks, the party must authenticate here by
performing a challenge-response protocol during the revision request. Putting
this challenge-response requirement in the revision-request can help improve
privacy, though the host is under no cryptographic or incentive-based
obligation to preserve the privacy of the revision.

1. The renter makes an RPC to the host, opening a connection. The connection
   deadline should be at least 120 seconds. The renter sends the file contract
   id for the revision being requested.

2. The host writes 32 bytes of random data that the renter must sign for the
   renter key in the corresponding file contract.

3. The renter returns the signed challenge.

4. The host verifies the signature from the renter and then sends the renter
   the most recent file contract revision, along with the transaction
   signatures from both the renter and the host. The connection is then closed.

File Contract Creation
----------------------

A few decisions were made regarding the file contract protocol. The first is
that the renter should not sign the file contract until the host has formally
accepted the file contract. The second is that the host should be the last one
to sign the file contract, as the renter is the party with the strong
reputation system.

Instead of sending a whole transaction each time, the transaction is sent
piecemeal, and only the new parts at each step are sent to the other party.
This minimizes the surface area of data for a malicious party to manipulate,
which means less verification code, which means less chances of having a bug in
the verification code.

The renter pays for the siafund fee on the host's collateral and contract fee.
If a renter opens a file contract and then never uses it, the host does not
lose money. This does put the renter at risk, as they may open up a file
contract and then watch the host leave, but the renter is spreading the risk
over communications with many hosts, and has a reputation system that will help
ensure that the renter is only dealing with upstanding hosts.

1. The renter makes an RPC to the host, opening a connection. The connection
   deadline should be at least 360 seconds.

2. The host sends the renter the most recent copy of its settings, signed. If
   the host is not accepting new file contracts, the connection is closed.

3. The renter sends a notice of acceptance or rejection. If the renter accepts,
   the renter then sends a funded file contract transaction without a
   signature, followed by the public key that will be used to create the
   renter's portion of the UnlockConditions for the file contract.

4. The host sends an acceptance or rejection of the file contract. If the host
   accepts, the host will add collateral to the file contract, and will send
   the renter the inputs + outputs for the collateral, followed by any new
   parent transactions. The length of any of these may be zero.

5. The renter indicates acceptance or rejection of the file contract. If the
   renter accepts, the renter will sign the file contract and send the
   transaction signatures to the host. The renter will also send a signature
   for a no-op file contract revision that follows the file contract.

6. The host may only reject the file contract in the event that the renter has
   sent invalid signatures, so the acceptance step is skipped. The host signs
   the file contract and sends the transaction signatures to the renter, and
   the host creates and sends a signature for the no-op revision that follows
   the file contract. The connection is closed.

File Contract Revision
----------------------

1. The renter makes an RPC to the host, opening a connection. The minimum
   deadline for the connection is 600 seconds. The renter then sends a file
   contract ID, indicating the file contract that is getting revised during the
   RPC.

2. The host will respond with a 32 byte challenge - a random 32 bytes that the
   renter will need to sign.

3. The renter will sign the challenge with the renter key that protects the
   file contract. This is to prove that the renter has access to the file
   contract.

4. The host will verify the challenge signature, then send an acceptance or
   rejection. If accetped, the host will send the most recent file contract
   revision for the file contract along with the transaction signagtures that
   validate the revision. The host will lock the file contract, meaning no
   other changes can be made to the revision file contract until this
   connection has closed.

   A loop begins. The host sends the most recent revision of the host settings
   to the renter, signed. The settings are sent after each iteration of the
   loop to enable high resolution dynamic pricing for the host, especially for
   bandwidth.

6. The renter may reject or accept the settings + revision. A specific
   rejection message will gracefully terminate the loop here. The renter will
   send an unsigned file contract revision followed by a batch of modification
   actions which the revision pays for. Batching allows the renter to send a
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

7. The host indicates either acceptance or rejection of the new revision.

8. The renter signs the revision and sends the signature to the host.

9. The host signs the revision and sends the signature to the renter. Both
   parties submit the new revision to the transaction pool. The connection
   deadline is reset to 600 seconds (unless the maximum deadline has been
   reached), and the loop restarts.

File Contract Renewal
---------------------

1. The renter makes an RPC to the host, opening a connection. The minimum
   deadline for the connection is 600 seconds. The renter then sends a file
   contract ID, indicating the file contract that is getting revised during the
   RPC.

2. The host will respond with a 32 byte challenge - a random 32 bytes that the
   renter will need to sign.

3. The renter will sign the challenge with the renter key that protects the
   file contract. This is to prove that the renter has access to the file
   contract.

4. The host will verify the challenge signature, then send an acceptance or
   rejection. If accetped, the host will send the most recent file contract
   revision for the file contract along with the transaction signagtures that
   validate the revision. The host will lock the file contract, meaning no
   other changes can be made to the revision file contract until this
   connection has closed. The host sends the most recent revision of the host
   settings to the renter, signed. If the host is not accepting new file
   contracts, the connection is closed.

5. The renter either accepts or rejects the settings. If accepted, the renter
   sends a funded, unsigned file contract to the host, containing the same
   Merkle root as the previous file contract, and also containing a renewed
   payout with conditional payments to the host to cover the host storing the
   data for the extended duration.

6. The host will accept or reject the renewed file contract. If accepted, the
   host will add collateral (and miner fees if desired) and send the inputs +
   outputs for the collateral, along with any new parent transactions. The
   length of any of these may be zero.

7. The renter will accept or reject the host's additions. If accepting, the
   renter will send signatures for the transaction to the host. The renter will
   also send a signature for a no-op file contract revision that follows the
   file contract.

8. The host may only reject the file contract in the event that the renter has
   sent invalid signatures, so the acceptance step is skipped. The host signs
   the file contract and sends the transaction signatures to the renter, and
   the host creates and sends a signature for the no-op revision that follows
   the file contract. The connection is closed.

Data Request
------------

1. The renter makes an RPC to the host, opening a connection. The connection
   deadline is at least 600 seconds. The renter will send a file contract id
   corresponding to the file contract that will be used to pay for the
   download.

2. The host will respond with a 32 byte challenge - a random 32 bytes that the
   renter will need to sign.

3. The renter will sign the challenge with the public key that protects the
   file contract being used to pay for the download. This proves that the
   renter has access to the payment.

4. The host will verify the challenge signature, and then send an acceptance or
   rejection. If accepted, the host will send the most recent file contract
   revision followed by the signautres that validate the revision. The host
   will lock the file contract, preventing other connections from making
   changes to the underlying storage obligation.

   A loop begins. The host sends the most recent external settings to the
   renter, signed. The settings are sent each iteration to provide high
   resolution dynamic bandwidth pricing.

5. The host will send the renter the most recent file contract revision, along
   with the signatures that validate the revision.

   A loop begins, which will allow the renter to download multiple batches of
   data from the same connection. The host will send the host settings, and the
   most recent file contract revision transaction. If there is no revision yet,
   the host will send a blank transaction. The host is expected to always have
   the most recent revision (the host signs last), the renter may not have the
   most recent revision.

6. The renter will accept or reject the host's settings. If accepting, the
   renter will send a file contract revision, unsigned, to pay for the download
   request. The renter will then send the download request itself.

7. The host will either accept or reject the revision.

8. The renter will send a signature for the file contract revision.

9. The host sends a signature for the file contract revision, followed by the
   data that was requested by the download request. The loop starts over, and
   the connection deadline is reset to a minimum of 600 seconds.
