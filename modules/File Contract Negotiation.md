File Contract Negotitation
==========================

Two untrusting parties, a renter and a host, need to fund a file contract and
put it in the blockchain. The following is a protocol for negotiating and
submitting such a contract without either party being at risk of losing coins.

1. The renter calls the `NegotiateContract` RPC on the host and opens up a
connection.

2. The renter sends the host a `ContractTerms` object containing terms for a
potential file contract.

3. The host can accept the contract by replying with the
`AcceptBontractResponse`. If the host does not agree with any part of the
terms, the host can instead write an error and close the connection.

4. The renter then sends the data that the host will be storing. The data must
be equal in length to the size indicated in the contract terms.

5. The renter creates a transaction containing the file contract. The renter
funds the transaction, but does not sign the transaction. Then the renter sends
the transaction to the host. At this point, the only risk that the renter has
taken is the resources expended to upload the data to the host.

6. The host inspects the data and the transaction. The transaction must contain
a file contract, and the merkle root specified in the contract must match the
merkle root of the data that the renter uploaded. All of the parts of the file
contract must match the contract terms that the renter sent, and the funding in
the transaction must match the renter contribution specified in the contract
terms. If anything is awry, the host writes an error and closes the connection.
At this point, the only risk is the resources the host spent downloading the
data from the renter.

The host adds any required collateral to the transaction, does not sign the
transaction, then sends it back to the renter.

7. The renter checks that the transaction sent by the host has not been changed
except for having more funds added to fund the host collateral. If anything
else has changed, the renter writes an error and closes the connection. No
additional risk has been added to either party at this point. Except for the
signatures, the transaction is now complete.

The renter grabs the ID of the file contract for later use. Then the renter
signs the whole transaction (preventing the host from changing anything without
invaliding the renter signature) and sends the signed transaction to the host.

8. The host checks once again that the transaction has not been changed except
for being signed. If something has changed, the host writes an error and closes
the connection. Otherwise, the host signs the transaction and submits it to the
blockchain.

9. Each party now must watch the blockchain to be certain that the file
contract has been accepted into the consensus set. If the file contract makes
it into the blockchain, each party is content, and the job has been completed.
If the transaction does not make it into the blockchain after some threshold of
blocks, each party can assume malice and must take steps to recover the funds
put into the file contract. Each party can guarantee their refund by submitting
a double spend of the inputs funding the file contract transaction. Once the
double spend has been submitted, the submitter needs only wait until the double
spend has been fully confirmed by the blockchain. The double spend can only be
foiled by the appearance of the file contract, which was the original goal
anyway.

Weaknesses 
----------

This protocol can fail if transactions are being actively censored by miners,
or are not being submitted with high enough fees to get priority on the
blockchain.
