This file is meant to help a developer navagate the codebase and develop clean,
maintainable code.

-------------------------------
-- Documentation Conventions --
-------------------------------

All structs, functions, and interfaces must have a docstring.

Anytime that something is left unfinished, place a comment containing the
string 'TODO:'. This sends a clear message to other developers, and creates a
greppable way to find unfinished parts of the codebase. Currently, it is okay
to leave a large volume of 'TODO' statements in the codebase. As the codebase
matures, 'TODO' statements will become increasingly frowned upon.

A softer form of 'TODO' is 'CONTRIBUTE:', which indicates a place in the
codebase that could use additional code, but it is only a 'would be nice', and
is not a high enough priority to actually be implemented. It is meant to
indicate to other developers (especially those new to the codebase) places that
would be easy contribute to.

---------------------------
-- Consensus Conventions --
---------------------------

It is convention to use the term 'apply' any time that a function is changing
the state as the result of information being introduced to the state. For
example 'ApplyBlock' and 'applyStorageProof'.

It is convention to use the term 'invert' any time that a function is changing
the state as the result of information being removed from the blockchain. For
example 'invertBlock'.

It is convention to use the term 'valid' any time that the validity of
something is being checked. It is convention not to change the state in any
capacity when inside of a validation function. For example 'validInput' and
'validContract'.

----------------------
-- Mutex Convetions --
----------------------

WARNING, TODO: These conventions are not currently followed closely in the
codebase. This is a bug, and needs to be fixed.

It is convention that any exported function will lock any state object that is
being manipulated. This means that exported functions can be called
concurrently without the caller needing to worry about locking or unlocking the
state object. Where possible, the mutex will be handled entirely at the top of
the function, via `Lock; defer Unlock;`. If the function does not handle
mutexes following this convention, it must be documented in the function
explanation.

It is convention that non-exported functions will not lock any state object,
and that their caller will manage the locks for them. This is because
non-exported are typically only called by exported functions, who will manage
the locks already in all-encompassing `lock; defer unlock` calls. If there is a
non exported function that breaks this convention (for example, a listen()
function), it must be documented in the function explanation.

--------------------
-- Consensus Flow --
--------------------

WARNING: This is both no longer fully correct and also not the flow that is
going to be upheld in the future. Better strategies have been learned and will
be implemented in a later iteration, after the Jan. 16th release.

Verifying and Applying a Block:

1. Check lists of blocks to see if the block is already known to the program.
2. Check that the block meets the target.
3. Check that the block is not in the future, and call sleep() if it is.
4. Check that the block is not in the past.
5. Check that the transaction hash matches the transaction list.
6. Add the block to the list of blocks, creating a new fork.
7. Broadcast the block to all known peers.
8. If the new fork is not heavier than the heaviest known fork, stop here.
9. Rewind blocks from the current fork until a common parent with the new fork is found.
10. For each block that needs to be added, repeat the remaining steps.
11. If a block fails, add it and all of its children (recursively) to the bad blocks list, rewind back to the common parent, and then re-apply all the blocks to restore the state to the old haviest fork.
12. Verify and then apply each transaction, adding up the total volume of miner fees in each.
13 Perform Contract Maintenance
	13a. Create any outputs from contracts with missed storage proofs.
	13b. Create any outputs from terminated contracts, and then delete the contract.
	13c. Update contract states to reset any storage proof windows that have progressed.
14. Add coin inflation to the miner subsidy, and create an output for the miner subsidy.
15. Update the values for the current block and current path.

Verifying a Transaction:

1. Sum all of the inputs
2. Check that all inputs spend existing outputs.
3. Check that the spend conditions for each input match the hash of the output they spend.
4. Check that the timelock on each output has expired.
5. Check that no inputs are spent twice.
6. Add up miner fees, outputs, and contract funds, make sure that is less than the sum of all inputs.
7. Make sure there are no illegal values in the contracts.
8. Make sure all storage proofs are valid.
9. Make sure all signatures are non-frivilous and valid.
10. Make sure each input has fully satisfied its signature requirements.

Applying a Transaction:

1. Remove the transaction from the transaction pool.
2. Delete all inputs from the unspent outputs list.
3. Add all financial outputs to the unspent outputs list.
4. Add all outputs created by the storage proofs.
5. Add all open contracts created by the file contracts.
6. (scan arbitrary data to fill out the host db)

Removing a Block:

1. Remove the output responsible for the miner subsidy.
2. Perform Inverse Contract Maintenance
	2a. Update contract states to set any storage proof windows that have retreated to the previous window.
	2b. Remove any outputs created from terminated contracts, and then restore the contract.
	2c. Remove any outputs created from missed storage proofs.
3. Remove each transaction that was in the block.
4. Update the values for the current block and current path.

Removing a Transaction:

1. (scan arbitrary data and remove any hosts from the host db)
2. Delete all open contracts created by the file contracts.
3. Delete all outputs created by the storage proofs.
4. Delete all financial outputs created by the transaction.
5. Restore all transaction inputs to the unspent outputs list.
6. Add the transaction back to the transaction pool.
