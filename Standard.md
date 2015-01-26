Some transactions will not be accepted by miners unless they appear in a block.
This is equivalent to the 'IsStandard' function in Bitcoin. This file dictates
the rules for standard Sia transactions.

-------------------------
-- Storage Proof Rules --
-------------------------

All storage proofs must be in their own transactions. One storage proof per
transaction, and if there is a storage proof in a transaction, it must not
contain any inputs, contracts, outputs, etc.

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
