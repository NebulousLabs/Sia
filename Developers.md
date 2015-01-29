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

Documentation should give a sense of what each function does, but should also
give a sense of the overall architecture of the code. Where useful, examples
should be provided, and common pitfalls should be explained.

------------------------
-- Naming Conventions --
------------------------

Names are used to give readers and reviers a sense of what is happening in the
code. When naming variables, you should assume that the person reading your
code is unfamiliar with the codebase. Short names (like 's' instead of 'state')
should only be used when the context is immediately obvious. For example
's := new(consensus.State)' is immediately obvious context for 's', and so 's'
is appropriate for the rest of the function.

When calling functions with obscure parameters, named variables should be used
to indicate what the parameters do. For example, 'm := NewMiner(1)' is
considered bad. Instead, use 'threads := 1; m := NewMiner(threads)'. The name
gives readers a sense of what the parameter within 'NewMiner' does even when
they are not familiar with the 'NewMiner' function.

----------------------
-- Mutex Convetions --
----------------------

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

Functions prefixed 'threaded' (example 'threadedMine') are meant to be called
in their own goroutine ('go threadedMine()') and will manage their own mutexes.
These functions typically loop forever, either listening on a channel or
performing some regular task, and should not be called with a mutex locked.

-------------------
-- Sanity Checks --
-------------------

Some functions make assumptions. For example, the 'addTransaction' function
assumes that the transaction being added is not in conflict with any other
transactions. Where possible, these explicit assumptions should be validated.

Example:

if consensus.DEBUG {
	_, exists := tp.usedOutputs[input.OutputID]
	if exists {
		panic("incorrect use of addTransaction")
	}
}

In the example, a panic is called for incorrect use of the function, but only
in debug mode. This failure will be invisible in production code, but the code
will have higher performace because the code should never fail anyway.

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
