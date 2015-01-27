Some transactions will not be accepted by miners unless they appear in a block.
This is equivalent to the 'IsStandard' function in Bitcoin. This file dictates
the rules for standard Sia transactions.

-------------------------
-- Storage Proof Rules --
-------------------------

Storage Proof transactions should not have dependent transactions.  Meaning,
any outputs that are spent with a storage proof should already by confirmed by
the blockchain, and any refunds or outputs created by the storage proof should
not be spent until the storage proof has been confirmed by the blockchain.

These restrictions are in place because storage proofs can be easily
invalidated by a blockchain reorg - if the trigger block changes, the proof
will be invalidated. This will screw up all dependents.

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
