Some transactions will not be accepted by miners unless they appear in a block.
This is equivalent to the 'IsStandard' function in Bitcoin. This file dictates
the rules for standard Sia transactions.

----------------------
-- Transaction Size --
----------------------

Consensus rules limit the size of a block, but not the size of a transaction.
Standard rules however limit the size of a single transaction to 64kb.

------------------------
-- Double Spend Rules --
------------------------

When two conflicting transactions are seen, the first transaction is the only
one that is kept. If the blockchain reorganizes, the transaction that is kept
is the transaction that was most recently in the blockchain. This is to
discourage double spending, and enforce that the first transaction seen is the
one that should be kept by the network.

Transactions are currently included into blocks using a first-come first-serve
algorithm. Eventually, transactions will be rejected if the fee does not meet a
certain minimum. For the near future, there are no plans to prioritize
transactions with substantially higher fees. Other mining software may take
alternative approaches.

-------------------------
-- Storage Proof Rules --
-------------------------

Storage Proof transactions should not have dependent transactions.  Meaning,
any outputs created in a storage proof transaction should not be spent until
the storage proof is confirmed by the blockchain.

These restrictions are in place because storage proofs can be easily
invalidated by a blockchain reorg - if the trigger block changes, the proof
will be invalidated. Storage proofs can by any reorg, where standard
transactions can only be invalidated by a doublespend (which requires a
signature from the double spender).

This also means that transaction pools will track multiple conflicting storage
proofs. If there are two competing reorgs, it is in the best interest of the
network to keep storage proofs for each reorg, because each proof will only be
valid on one reorg.

--------------------------
-- Arbitrary Data Usage --
--------------------------

Arbitrary data can be used to make verifiable announcements, or to have other
protocols sit on top of Sia. The arbitrary data can also be used for soft
forks, and for protocol relevant information. Any arbitrary data is allowed by
consensus, but only certain arbitrary data is considered standard.

Arbitrary data that is prefixed by the string 'NonSia' is always allowed. This
indicates that the remaining data has no relevance to Sia protocol rules, and
never will.

Arbitrary data that is prefixed by the string 'HostAnnouncement' is allowed,
but only if the data within accurately decodes to the HostAnnouncement struct
found in modules/hostdb.go, and contains no extra information.
