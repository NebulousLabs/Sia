This file documents the rules of consensus in the Sia protocol.

----------------------
-- Block Timestamps --
----------------------

Each block has a minimum allowed timestamp. The minumum timestamp is found by
taking the median timestamp of the previous 11 blocks. If there are not 11
previous blocks, the genesis timestamp is used repeatedly.

------------------
-- Block Target --
------------------

Each block has a target, which is a value that the hash of the block must be
lower than in order for the block to be valid. A new target is set every block.
The target is set by comparing the timestamp of the current block with the
timestamp of the block added 2000 blocks prior. The expected difference in time
is 20,000 minutes. If less time has passed, the target is lowered. If more time
has passed, the target is increased.

The target is changed in proportion to the difference in time (If the time was
half of what was expected, the new target is 1/2 the old target). There is a
clamp on the adjustment. In one block, the target cannot adjust upwards by more
more than 1001/1000, and cannot adjust downwards by more than 999/1000.

If there are not 2000 blocks, the genesis timestamp is used as the moment to
compare to. The expected time is 10 minutes * (block height).

------------------
-- Transactions --
------------------

A transaction is valid if:
	+ All inputs are fully signed, without frivilous signatures
	+ All inputs spend known outputs
	+ The inputs equal the outputs
		- Outputs can be outputs
		- Miner Fees can be outputs
		- File Contracts can be outputs
	+ All contracts have a non-zero payout
	+ All storage proofs act on contracts found in the blockchain
	+ All storage proofs are correct
