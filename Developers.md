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
