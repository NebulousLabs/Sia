Standard Transaction Rules
==========================

Some transactions will not be accepted by miners unless they appear in a block.
This is equivalent to the 'IsStandard' function in Bitcoin. This file dictates
the rules for standard Sia transactions.

Transaction Size
----------------

Consensus rules limit the size of a block, but not the size of a transaction.
Standard rules however limit the size of a single transaction to 16kb.

A chain of dependent transactions cannot exceed 500kb.

Double Spend Rules
------------------

When two conflicting transactions are seen, the first transaction is the only
one that is kept. If the blockchain reorganizes, the transaction that is kept
is the transaction that was most recently in the blockchain. This is to
discourage double spending, and enforce that the first transaction seen is the
one that should be kept by the network. Other conflicts are thrown out.

Transactions are currently included into blocks using a first-come first-serve
algorithm. Eventually, transactions will be rejected if the fee does not meet a
certain minimum. For the near future, there are no plans to prioritize
transactions with substantially higher fees. Other mining software may take
alternative approaches.

File Contract Rules
-------------------

File Contracts that start in less than 10 blocks time are not accepted into the
transaction pool. This is because a file contract becomes invalid if it is not
accepted into the blockchain by the start block, and this might result in a
cascade of invalidated unconfirmed transactions, which may make it easier to
launch double spend attacks on zero confirmation outputs. 10 blocks is plenty
of time on the other hand for a file contract to make it into the blockchain.

Signature Algorithms
--------------------

Miners will reject transactions that have public keys using algorithms that the
miner does not understand.

Arbitrary Data Usage
--------------------

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
