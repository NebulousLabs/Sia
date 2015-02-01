Consensus Rules
===============

Sia is a cryptosystem that uses a Bitcoin style Proof of Work blockchain to
achieve network consensus. The rules of consensus are detailed in this
document. The most detailed and accurate explanation however is the codebase
itself. If you are looking for a more precise understanding of the consensus
rules, a good place to start is consensus/types.go, and another good starting
place is the function AcceptBlock, which can be found in consensus/blocks.go.

This document will be more understandable if you have a general understanding
of proof of work blockchains, and does not try to build up from first
priciples.

TODO: Write the formal specification for encoding things.

TODO: Write the formal specification for deriving the block id, contract id,
siacoin output id, siafund output id, siafund claim output id.

TODO: block id + merkle root, miner output ids, siacoin output ids, contract
ids, storage proof ids, siafund output ids, siafund claim output ids.

TODO: Document which crypto is used in consensus. (Hash algorithm, signature
algorithm)

TODO: Pick a timestamp for the genesis block.

Currency
--------

The Sia cryptosystem has two types of currency. The first is the Siacoin.
Siacoins are generated every block and distributed to the miners. These miners
can then use the siacoins to fund file contracts, or can send the siacoins to
other parties (presumably in a trade for out-of-band goods and services, or
simply other currencies.). The siacoin is represented by a 128 bit unsigned
integer.

The second currency in the Sia cryptosystem is the Siafund, which is a special
asset limited to 10,000 indivisible units. Each time a contract is created,
3.9% of the payout is put into the siafund pool. The number of siacoins in the
siafund pool must always be divisible by 10,000; the number of coins taken from
the payout is rounded down to the nearest 10,000.

Siafund owners can collect the siacoins in the siafund pool. For every 10,000
siacoins added to the siafund pool, a siafund owner can withdraw 1 siacoin.
Approx. 8750 siafunds are owned by Nebulous Inc. The remaining siafunds are
owned by early backers of the Sia project.

There are future plans to enable sidechain compatibility with Sia. This would
allow other currencies such as Bitcoin to be spent in all the same places that
the Siacoin can be spent.

Block Size
----------

The maximum block size is 1024 * 1024 bytes. There is no limit on transaction
size, though it must fit inside of the block.

Block Timestamps
----------------

Each block has a minimum allowed timestamp. The minumum timestamp is found by
taking the median timestamp of the previous 11 blocks. If there are not 11
previous blocks, the genesis timestamp is used repeatedly.

Blocks will be rejected if they are timestamped more than three hours in the
future, but can be accepted again once enough time has passed.

Block ID
--------

The ID of a block is derived using:
	Hash(Parent Block ID + 64 bit Nonce + Block Merkle Root)

The block merkle root is obtained by creating a merkle tree whose leaves are
the hash of the timestamp, the hashes of the miner outputs (one leaf per miner
output), and the hashes of the transactions (one leaf per transaction).

Block Target
------------

For a block to be valid, the id of the block must be below a certain target.  A
new target is set every block by by comparing the timestamp of the current
block with the timestamp of the block added 2000 blocks prior. The expected
difference in time is 20,000 minutes. If less time has passed, the target is
lowered. If more time has passed, the target is increased.

The target is changed in proportion to the difference in time (If the time was
half of what was expected, the new target is 1/2 the old target). There is a
clamp on the adjustment. In one block, the target cannot adjust upwards by more
more than 1001/1000, and cannot adjust downwards by more than 999/1000.

The new target is calculated using (expected time passed in seconds) / (actual
time passed in seconds) * (current target). The division and multiplication
should be done using infinite precision, and the result should be truncated.

If there are not 2000 blocks, the genesis timestamp is used for comparison.
The expected time is (10 minutes * block height).

The difficulty clamp means that the target can shift by at most 7.5x in 2016
blocks, which can be compared to the 4x clamp of Bitcoin. The amount of work
required to quadruple the difficulty in Sia is 3000x the starting difficulty,
which can be compared to 2016x for Bitcoin. The amount of work required to 16x
the difficulty in Sia is 15,000x the original difficulty, which can be compared
to 10,000x for Bitcoin.

Block Subsidy
-------------

The coinbase for a block is (300,000 - (1 * height)) * 2^80, with a minimum of
30,000 * 2^80. Any miner fees get added to the coinbase, which creates the
block subsidy. The block subsidy is then given to multiple outputs, called the
miner payouts. The total value of the miner payouts must equal the block
subsidy.  Having multiple outputs allows the block reward to be sent to
multiple people, enabling systems like p2pool.

The ids of the outputs created by the miner payouts is determined by taking the
block id and concatenating the index of the payout that the output corresponds
to.

The outputs created by the block subsidy cannot be spent for 100 blocks, and
are not considered a part of the utxo set until 100 blocks have transpired.
This limitation is in place because a simple reorg is enough to invalidate the
output; double spend attacks and false spend attacks are much easier.

Transactions
------------

A Transaction is composed of a set of inputs, miner fees, outputs, file
contracts, storage proofs, arbitrary data, and signatures. The sum of the
inputs must equal the sum of the miner fees, outputs, and contract payouts.

A Transaction is composed of the following:
- Inputs
- Miner Fees
- Outputs
- File Contracts
- Storage Proofs
- Siafund Inputs
- Siafund Outputs
- Arbitrary Data
- Signatures

The financial sum of all the inputs must equal the financial sum of all the
miner fees, outputs, and contract payouts. There can be no leftovers.

The financial sum of all siafund inputs must equal the financial sum of all
siafund outputs.

Inputs
------

Each input spends an output.  The output being spent must already exist in the
state. An output has a value, and a spend hash (or address), which is the hash
of the 'spend conditions' object of the output. The spend conditions contain a
timelock, a number of required signatures, and a set of public keys that can be
used during signing. The input is invalid if hash of the spend conditions do
not match the spend hash of the output being spent.

The timelock is a block height, and for the input to be valid, the current
height of the blockchain must be at least the height stated in the timelock.

There is a list of public keys which can each be used at most once when signing
a transaction. The same public key can be listed twice, which means that it can
be used twice. The number of required signatures indicates how many public keys
must be used to validate the input. If required signatures is '0', the input is
effectively 'anyone can spend'. If the required signature count is greater than
the number of public keys, the input is unspendable.

An input must have exactly the right number of signatures. Extra signatures are
not allowed.

Miner Fees
----------

A miner fee is an output that gets added directly to the block subsidy.

Outputs
-------

Outputs contain a value and a spend hash (also called a coin address). The
spend hash is a hash of the spend conditions that must be met to spend the
output.

File Contracts
--------------

A file contract is an agreement by some party to prove they have a file at a
given point in time. The contract contains the merkle root of the data being
stored, and the size in bytes of the data being stored.

The merkle root is formed by breaking the file into 64 byte segments and
hashing each segment to form the leaves of the merkle tree. The final segment
is not padded out.

The storage proof must be submitted between the 'Start' and 'End' fields of the
contract. There is a 'Payout', which indicates how many coins are given out
when the storage proof is provided. If the storage proof is provides, the
payout goes to 'ValidProofAddress'. If no proof is submitted by block height
'End', then the payout goes to 'MissedProofAddress'.

All contracts must have a non-zero payout.

Storage Proofs
--------------

A storage proof transaction is any transaction containing a storage proof. 

Storage proof transactions are not allowed to have siacoin or siafund outputs,
and are not allowed to have file contracts. All outputs created by storage
proofs cannot be spent for 100 blocks.

When creating a storage proof, you only prove that you have a single 64 byte
segment of the file. The piece that you must prove you have is chosen
psuedorandomly using the contract id and the id of the 'trigger block'.  The
trigger block is the block at height 'Start' - 1, where 'Start' is the value
'Start' in the contract that the storage proof is fulfilling.

The file is composed of 64 byte segments whose hashes compose the leaves of a
merkle tree. When proving you have the file, you must prove you have one of the
leaves. To determine which leave, take the hash of the contract id concatenated
to the trigger block id, then take the numerical value of the result modulus
the number of segments:

	Hash(contract id + trigger block id) % num segments

The proof is formed by providing the 64 byte segment, and then the missing
hashes required to fill out the remaining tree. The total size of the proof
will be 64 bytes + 32 bytes * log(num segments), and can be verified by anybody
who knows the root hash and the file size.

Storage proof transactions are not allowed to have siacoin outputs, siafund
outputs, or contracts. All outputs created by the storage proofs cannot be
spent for 100 blocks.

These limits are in place because a simple blockchain reorganization can change
the trigger block, which will invalidate the storage proof and therefore the
entire transaction. This makes double spend attacks and false spend attacks
significantly easier to execute.

Siafund Inputs
--------------

A siafund input works similar to a siacoin input. It contains the id of a
siafund output being spent, and the spend conditions required to spend the
output. These spend conditions must be signed.

A special output is created when a siafund is used as input. All of the
siacoins that have accrued in the siafund since its last spend are sent to the
'Claim Destination' found in the siafund output, which is a normal siacoin
address. The number of accrued siacoins is counted by taking the size of the
siacoin pool when the output was created and comparing it to the current size
of the siacoin pool. The equation is:

	(Current Pool Size - Previous Pool Size) / 10,000 * siafund quantity

Like the miner outputs and the storage proof outputs, the siafund output cannot
be spent for 100 blocks because the value of the output can change if the
blockchain reorganizes. Reorganizations will not however cause the transaction
to be invalidated, so the ban on contracts and outputs does not need to be in
place.

Siafund Outputs
---------------

Sia outputs contain:
- A Value, which is the quanity of siafunds controlled by the output.
- A Spend Hash, which is the hash of the spend conditions required to spend the
  siafund output.
- A Claim Destination, which is a siacoin address to which siacoins will be
  sent when the siafund output is spent.
- A Claim Start, which indicates how many siacoins were in the siafund pool at
  the moment the siafund output got created. This is used when the output is
  spent to determine how many siacoins go to the new output.

Arbitrary Data
--------------

Arbitrary data is a set of data that is ignored by consensus. In the future, it
may be used for soft forks, paired with 'anyone can spend' transactions. In the
meantime, it is an easy way for third party applications to make use of the
siacoin blockchain. Bloat and spam is combatted using fee requirements.

Signatures
----------

Each signature points to a single public key index in a single input. No two
signatures can point to the same public key index for the same input. The
signature can point to either a siacoin input or a siafund input.

Each signature also contains a timelock, and is not valid until the blockchain
has reached a height equal to the timelock height.

Signatures also have a 'Covered Fields' struct, which indicates which parts of
the transaction get included in the signature. There is a 'whole transaction'
flag, which indicates that every part of the transaction except for the
signatures gets included, which eliminates any malleability outside of the
signatures. The signatures can also be individually included, to enforce that
your signature is only valid if certain other sigantures are present.

If the 'whole transaction' is not set, all fields need to be added manually,
and additional parties can add new fields, meaning the transaction will be
malleable. This does however allow other parites to add additional inputs,
fees, etc. after you have signed the transaction without invalidating your
signature.

Entirely nonmalleable transactions can be achieved by setting the 'whole
transaction' flag and then providing the last signature, including every other
signature in your signature. Because no frivilous signatures are allowed, the
transaction cannot be changed without your signature being invalidated.

Consensus Set
-------------

The blockchain is used to achieve consensus around 3 objects. The first is
unspent financial outputs. The second is unfulfilled storage contracts. The
third is siafund ownership and claims. All transaction componenets have some
effect on the three sets of information.

Genesis Set
-----------

The genesis block will be a block with a timestamp of TODO:TBD. All other
fields will be empty. The required target for the next block shall be [0, 0, 0,
8, 0...], where each value is a byte.

The genesis block does not need to meet a particular target, and indeed it does
not.
