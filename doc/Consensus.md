Consensus Rules
===============

This document is meant to provide a good high level overview of the Sia
cryptosystem, but does not fully explain all of the small details. The most
accurate explanation of the consensus rules is the consensus package.

This document will be more understandable if you have a general understanding
of proof of work blockchains, and does not try to build up from first
principles.

If you wish to know more about the protocol and the consensus rules, you should
read the documentation in the following order:

- Consensus.md
- consensus/types.go
- consensus/state.go

From there, you can start reading through the logic of the consensus package,
starting at the function AcceptBlock in consensus/blocks.go.

Cryptographic Algorithms
------------------------

Sia uses cryptographic hashing and cryptographic signing, each of which has
many potentially secure algorithms that can be used. We acknowledge our
inexperience, and that we have chosen these algorithms not because of our own
confidence in their properties, but because other people seem confident in
their properties.

For hashing, our primary goal is to use an algorithm that cannot be merge mined
with Bitcoin, even partially. A secondary goal is hashing speed on consumer
hardware, including phones and other low power devices.

For signing, our primary goal is verification speed. A secondary goal is an
algorithm that supports HD keys. A tertiary goal is an algorithm that supports
threshold signatures.

#### Hashing: blake2b

  blake2b has been chosen as a hashing algorithm because it is fast, it has had
  substantial review, and it has invulnerability to length extension attacks.
  Another particularly important feature of BLAKE2b is that it is not SHA-2. We
  wish to avoid merge mining with Bitcoin, because that may result in many
  apathetic Bitcoin miners mining on our blockchain, which may make soft forks
  harder to coordinate.

#### Signatures: variable type signatures

  Each public key will have an specifier (a 16 byte array) and a byte slice
  containing an encoding of the public key. The specifier will tell the
  signature verification which signing algorithm to use when verifying a
  signature. Each signature will be a byte slice, the encoding can be
  determined by looking at the specifier of the corresponding public key.

  This method allows new signature types to be easily added to the currency in
  a way that does not invalidate existing outputs and keys. Adding a new
  signature type requires a hardfork, but allows easy protection against
  cryptographic breaks, and easy migration to new cryptography if there are any
  breakthroughs in areas like verification speed, ring signatures, etc.

  Allowed algorithms:
  
  ed25519: The specifier must match the string "ed25519". The public key
  must be encoded into 32 bytes. Signatures and public keys will need to
  follow the ed25519 specification. More information can be found at
  ed25519.cr.yp.to

  entropy: The specifier must match the string "entropy". The signature will
  always be invalid. This provides a way to add entropy buffers to
  SpendCondition objects to protect low entropy information, while being able
  to prove that the entropy buffers are invalid public keys.
  
  There are plans to also add ECDSA secp256k1 and Schnorr secp256k1. New
  signing algorithms can be added to Sia through a soft fork, because
  unrecognized algorithm types are always considered to have valid signatures.

Currency
--------

The Sia cryptosystem has two types of currency. The first is the Siacoin.
Siacoins are generated every block and distributed to the miners. These miners
can then use the siacoins to fund file contracts, or can send the siacoins to
other parties. The siacoin is represented by a 128 bit unsigned integer.

The second currency in the Sia cryptosystem is the Siafund, which is a special
asset limited to 10,000 indivisible units. Each time a file contract payout is
made, 3.9% of the payout is put into the siafund pool. The number of siacoins
in the siafund pool must always be divisible by 10,000; the number of coins
taken from the payout is rounded down to the nearest 10,000. The siafund is
also represented by a 128 bit unsigned integer, even though only 16 bits of
resolution are required.

Siafund owners can collect the siacoins in the siafund pool. For every 10,000
siacoins added to the siafund pool, a siafund owner can withdraw 1 siacoin.
Approx. 8750 siafunds are owned by Nebulous Inc. The remaining siafunds are
owned by early backers of the Sia project.

There are future plans to enable sidechain compatibility with Sia. This would
allow other currencies such as Bitcoin to be spent in all the same places that
the Siacoin can be spent.

Marshalling
-----------

Many of the Sia types need to be hashed at some point, which requires having a
consistent algorithm for marshalling types into a set of bytes that can be
hashed. The following rules are used for hashing:

 - Integers are little-endian, and are always encoded as 8 bytes.
 - Bools are encoded as one byte, where zero is false and one is true.
 - Variable length types such as strings are prefaced by 8 bytes containing
   their length.
 - Arrays and structs are encoded as their individual elements concatenated
   together. The ordering of the struct is determined by the struct definition.
   There is only one way to encode each struct.
 - The Currency type is encoded as a 16 byte unsigned integer.

Block Size
----------

The maximum block size is 1024 * 1024 bytes. There is no limit on transaction
size, though it must fit inside of the block.

Block Timestamps
----------------

Each block has a minimum allowed timestamp. The minimum timestamp is found by
taking the median timestamp of the previous 11 blocks. If there are not 11
previous blocks, the genesis timestamp is used repeatedly.

Blocks will be rejected if they are timestamped more than three hours in the
future, but can be accepted again once enough time has passed.

Block ID
--------

The ID of a block is derived using:
	Hash(Parent Block ID + 64 bit Nonce + Block Merkle Root)

The block Merkle root is obtained by creating a Merkle tree whose leaves are
the hash of the timestamp, the hashes of the miner outputs (one leaf per miner
output), and the hashes of the transactions (one leaf per transaction).

Block Target
------------

For a block to be valid, the id of the block must be below a certain target.  A
new target is set every block by by comparing the timestamp of the current
block with the timestamp of the block added 1000 blocks prior. The expected
difference in time is 10,000 minutes. If less time has passed, the target is
lowered. If more time has passed, the target is increased.

The target is changed in proportion to the difference in time (If the time was
half of what was expected, the new target is 1/2 the old target). There is a
clamp on the adjustment. In one block, the target cannot adjust upwards by more
more than 1001/1000, and cannot adjust downwards by more than 999/1000.

The new target is calculated using (expected time passed in seconds) / (actual
time passed in seconds) * (current target). The division and multiplication
should be done using infinite precision, and the result should be truncated.

If there are not 1000 blocks, the genesis timestamp is used for comparison.
The expected time is (10 minutes * block height).

The difficulty clamp means that the target can shift by at most 7.5x in 2016
blocks, which can be compared to the 4x clamp of Bitcoin. The amount of work
required to quadruple the difficulty in Sia is 3000x the starting difficulty,
which can be compared to 2000x for Bitcoin. The amount of work required to 16x
the difficulty in Sia is 15,000x the original difficulty, which can be compared
to 10,000x for Bitcoin.

Block Subsidy
-------------

The coinbase for a block is (300,000 - height) * 2^80, with a minimum of 30,000
\* 2^80. Any miner fees get added to the coinbase to create the block subsidy.
The block subsidy is then given to multiple outputs, called the miner payouts.
The total value of the miner payouts must equal the block subsidy. Having
multiple outputs allows the block reward to be sent to multiple people,
enabling systems like p2pool.

The ids of the outputs created by the miner payouts is determined by taking the
block id and concatenating the index of the payout that the output corresponds
to.

The outputs created by the block subsidy cannot be spent for 50 blocks, and are
not considered a part of the consensus set until 50 blocks have transpired.
This limitation is in place because a simple blockchain reorganization is
enough to invalidate the output; double spend attacks and false spend attacks
are much easier to execute.

Transactions
------------

A Transaction is composed of the following:

- Siacoin Inputs
- Miner Fees
- Siacoin Outputs
- File Contracts
- File Contract Terminations
- Storage Proofs
- Siafund Inputs
- Siafund Outputs
- Arbitrary Data
- Signatures

The sum of all the siacoin inputs must equal the sum of all the miner fees,
siacoin outputs, and contract payouts. There can be no leftovers. The sum of
all siafund inputs must equal the sum of all siafund outputs.

Several objects have unlock hashes. An unlock hash is the Merkle root of the
'unlock conditions' object. The unlock conditions contain a timelock, a number
of required signatures, and a set of public keys that can be used during
signing.

The Merkle root of the unlock condition objects is formed by taking the Merkle
root of a tree whose leaves are the timelock, the public keys (one leaf per
key), and the number of signatures. This ordering is chosen specifically
because the timelock and the number of signatures are low entropy. By using
random data as the first and last public key, you can make it safe to reveal
any of the public keys without revealing the low entropy items.

The unlock conditions cannot be satisfied until enough signatures have
provided, and until the height of the blockchain is at least equal to the value
of the timelock.

The unlock conditions contains a set of public keys which can each be used only
once when providing signatures. The same public key can be listed twice, which
means that it can be used twice. The number of required signatures indicates
how many public keys must be used to validate the input. If required signatures
is '0', the input is effectively 'anyone can spend'. If the required signature
count is greater than the number of public keys, the input is unspendable.
There must be exactly enough signatures. For example, if there are 3 public
keys and only two required signatures, then only two signatures can be included
into the transaction.

Siacoin Inputs
--------------

Each input spends an output. The output being spent must exist in the consensus
set. The 'value' field of the output indicates how many siacoins must be used
in the outputs of the transaction. Valid outputs are miner fees, siacoin
outputs, and contract payouts.

Miner Fees
----------

A miner fee is a volume of siacoins that get added to the block subsidy.

Siacoin Outputs
---------------

Siacoin outputs contain a value and an unlock hash (also called a coin
address). The unlock hash is the Merkle root of the spend conditions that must
be met to spend the output.

File Contracts
--------------

A file contract is an agreement by some party to prove they have a file at a
given point in time. The contract contains the Merkle root of the data being
stored, and the size in bytes of the data being stored.

The Merkle root is formed by breaking the file into 64 byte segments and
hashing each segment to form the leaves of the Merkle tree. The final segment
is not padded out.

The storage proof must be submitted between the 'start' and 'end' fields of the
contract. There is a 'payout', which indicates how many siacoins are given out
when the storage proof is provided. 3.9% of this payout (rounded down to the
nearest 10,000) is put aside for the owners of siafunds. If the storage proof
is provided and is valid, the remaining payout is put in an output spendable by
the 'valid proof spend hash', and if a valid storage proof is not provided to
the blockchain by 'end', the remaining payout is put in an output spendable by
the 'missed proof spend hash'.

All contracts must have a non-zero payout, 'start' must be before 'end', and
'start' must be greater than the current height of the blockchain. A storage
proof is acceptible if it is submitted in the block of height 'end'.

File contracts are created with a 'Termination Hash', which is the Merkle root
of an unlock conditions object. A 'file contract termination' can be submitted
which fulfills the unlock conditions object, resulting in the contract payout
being distributed according to the fields of the termination object, as opposed
to being distributed according to whether a valid storage proof was submitted
or not. This provides flexibility to edit an resubmit file contracts.

File Contract Terminations
--------------------------

A file contract termination voids a file contract, recovering the payout and
distributing it to a set of siacoin outputs that are specified in the
termination. The sum of the termination payouts must equal the value of the
original contract payout.

Storage Proofs
--------------

A storage proof transaction is any transaction containing a storage proof.
Storage proof transactions are not allowed to have siacoin or siafund outputs,
and are not allowed to have file contracts.

When creating a storage proof, you only prove that you have a single 64 byte
segment of the file. The piece that you must prove you have is chosen
randomly using the contract id and the id of the 'trigger block'.  The
trigger block is the block at height 'Start' - 1, where 'Start' is the value
'Start' in the contract that the storage proof is fulfilling.

The file is composed of 64 byte segments whose hashes compose the leaves of a
Merkle tree. When proving you have the file, you must prove you have one of the
leaves. To determine which leaf, take the hash of the contract id concatenated
to the trigger block id, then take the numerical value of the result modulus
the number of segments:

	Hash(contract id + trigger block id) % num segments

The proof is formed by providing the 64 byte segment, and then the missing
hashes required to fill out the remaining tree. The total size of the proof
will be 64 bytes + 32 bytes * log(num segments), and can be verified by anybody
who knows the root hash and the file size.

Storage proof transactions are not allowed to have siacoin outputs, siafund
outputs, or contracts. All outputs created by the storage proofs cannot be
spent for 50 blocks.

These limits are in place because a simple blockchain reorganization can change
the trigger block, which will invalidate the storage proof and therefore the
entire transaction. This makes double spend attacks and false spend attacks
significantly easier to execute.

Siafund Inputs
--------------

A siafund input works similar to a siacoin input. It contains the id of a
siafund output being spent, and the unlock conditions required to spend the
output.

A special output is created when a siafund output is used as input. All of the
siacoins that have accrued in the siafund since its last spend are sent to the
'claim spend hash' found in the siafund output, which is a normal siacoin
address. The value of the siacoin output is determined by taking the size of
the siacoin pool when the output was created and comparing it to the current
size of the siacoin pool. The equation is:

	((Current Pool Size - Previous Pool Size) / 10,000) * siafund quantity

Like the miner outputs and the storage proof outputs, the siafund output cannot
be spent for 50 blocks because the value of the output can change if the
blockchain reorganizes. Reorganizations will not however cause the transaction
to be invalidated, so the ban on contracts and outputs does not need to be in
place.

Siafund Outputs
---------------

Like siacoin outputs, siafund outputs contain a value and an unlock hash. The
value indicates the number of siafunds that are put into the output, and the
unlock hash is the Merkle root of the unlock conditions object which allows the
output to be spent.

Siafund outputs also contain a claim unlock hash field, which indicates the
unlock hash of the siacoin output that is created when the siafund output is
spent. The value of the output that gets created will depend on the growth of
the siacoin pool between the creation and the spending of the output. This
growth is measured by storing a 'claim start', which indicates the size of the
siafund pool at the moment the siafund output was created.

Arbitrary Data
--------------

Arbitrary data is a set of data that is ignored by consensus. In the future, it
may be used for soft forks, paired with 'anyone can spend' transactions. In the
meantime, it is an easy way for third party applications to make use of the
siacoin blockchain.

Signatures
----------

Each signature points to a single public key index in a single unlock
conditions object. No two signatures can point to the same public key index for
the same set of unlock conditions.

Each signature also contains a timelock, and is not valid until the blockchain
has reached a height equal to the timelock height.

Signatures also have a 'covered fields' object, which indicates which parts of
the transaction get included in the signature. There is a 'whole transaction'
flag, which indicates that every part of the transaction except for the
signatures gets included, which eliminates any malleability outside of the
signatures. The signatures can also be individually included, to enforce that
your signature is only valid if certain other signatures are present.

If the 'whole transaction' is not set, all fields need to be added manually,
and additional parties can add new fields, meaning the transaction will be
malleable. This does however allow other parties to add additional inputs,
fees, etc. after you have signed the transaction without invalidating your
signature. If the whole transaction flag is set, all other elements in the
covered fields object must be empty except for the signatures field.

The covered fields object contains a slice of indexes for each element of the
transaction (siacoin inputs, miner fees, etc.). The slice must be sorted, and
there can be no repeated elements.

Entirely nonmalleable transactions can be achieved by setting the 'whole
transaction' flag and then providing the last signature, including every other
signature in your signature. Because no frivolous signatures are allowed, the
transaction cannot be changed without your signature being invalidated.

Consensus Set
-------------

The blockchain is used to achieve consensus around 3 objects. The first is
unspent financial outputs. The second is unfulfilled storage contracts. The
third is siafund ownership and claims. All transaction components have some
effect on this set of information.

Genesis Set
-----------

The genesis block will have a unix timestamp set to 1427760000, which
corresponds to March 31st, 2015 at midnight.  All other fields will be empty.
The required target for the next block shall be [0, 0, 0, 1, 0...], where each
value is a byte.

The genesis block does not need to meet a particular target.

The genesis state needs to have an output to the zero address from the genesis
block, and a siafund output to the Nebulous Genesis Address for 10,000
siafunds (both the spend hash and the claim destination), having the zero id.
