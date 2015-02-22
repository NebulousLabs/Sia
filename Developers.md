Developer Environment
=====================

Sia is mostly written in go. To build and test Sia, you are going to need a
working go environment, including having both $GOROOT/bin and $GOPATH/bin in
your $PATH. For most Linux distributions, go will be in the package manager.
Then it should be sufficient to run `make dependencies && make`. For more
information, check the [go documentation](golang.org/doc/install).

If you plan on cross compiling Sia, you may need to install go from source. You
can find information on that [here](golang.org/doc/install/source)

Developer Conventions
=====================

This file is meant to help a developer navagate the codebase and develop clean,
maintainable code. Knowing all of these conventions will also make it easier to
read and code review the Sia project.

The primary purpose of the conventions within Sia is to keep the codebase
simple. Simpler constructions means easier code reviews, greater accessibility
to newcomers, and less potential for mistakes. It is also to keep things
uniform, much in the spirit of 'go fmt'. When everything looks the same,
everyone has an easier time reading and reviewing code they did not write
themselves.

Documentation
-------------

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
should be provided, and common pitfalls should be explained. Anything that
breaks other coventions in any way needs to have a comment, even if it is
obvious why the convention had to be broken.

The goal of the codebase is to be accessible to newbies. Anything more advanced
than what you would expect to remember from an 'Intro to Data Structures' class
should have an explanation about what the concept it is and why it was picked
over other potential choices.

Naming
------

Names are used to give readers and reviers a sense of what is happening in the
code. When naming variables, you should assume that the person reading your
code is unfamiliar with the codebase. Short names (like 's' instead of 'state')
should only be used when the context is immediately obvious. For example
's := new(consensus.State)' is immediately obvious context for 's', and so 's'
is appropriate for the rest of the function.

Data structures should never have shortened names. 'FileContract.mr' is
confusing to anyone who has not used the data structure extensively. The code
should be accessible to people who are unfamiliar with the codebase.

When calling functions with obscure parameters, named variables should be used
to indicate what the parameters do. For example, 'm := NewMiner(1)' is
confusing. Instead, use 'threads := 1; m := NewMiner(threads)'. The name gives
readers a sense of what the parameter within 'NewMiner' does even when they are
not familiar with the 'NewMiner' function.

The most important thing to remember when choosing names is to cater to people
who are unfamiliar with the code. A reader should never have to ask 'What is
`s`?' on their first pass through the code, even though to most of it is
painfully obvious that `s` refers to a consensus.State.

Control Flow
------------

Where possible, control structures should be minimized or avoided. This
includes avoiding nested if statements, and avoiding else statements where
possible. Sometimes, complex control structures are necessary, but where
possible use alternative code patterns and insert functions to break things up.

Example:

```go
// Do not do this:
if err != nil {
	return
} else {
	forkBlockchain(node)
}

// Instead to this:
if err != nil {
	return
}
forkBlockchain(node)
```

Mutexes
-------

Any exported function will lock the data structures it interacts with such that
the function can safely be called concurrently without the caller needing to
know anything about the threading. In particular, the function should have a
'Lock(); defer Unlock()' right at the top, or should otherwise have a comment
explaining why the mutex usage in the function breaks convention. Functions
that do not need to deal with mutexes at all do not need to mention mutexes in
the docstring.

Any non-exported functions will not lock the data structures they interact
with. The responsibility for locking comes from the exported functions. This
means that developers can safely assume the usage of non exported functions
will not cause deadlock within the program. This convention is strictly
enforced.

One exception to this rule is for functions with the prefix 'threaded' (example
'threadedMine'). The 'threaded' prefix indicates that the function should be
called in a separate goroutine and that the function will manage its own
mutexes. Deadlock is not a risk for callers in this case, because they know to
call the function in a separate goroutine. This also makes it easier for code
reviews to catch mistakes.

Functions prefixed 'threaded' (example 'threadedMine') are meant to be called
in their own goroutine ('go threadedMine()') and will manage their own mutexes.
These functions typically loop forever, either listening on a channel or
performing some regular task, and should not be called with a mutex locked.

Error Handling
--------------

All errors need to be checked as soon as they are received, even if they are
known to not cause problems. The statement that checks the error needs to be
`if err != nil`, and if there is a good reason to use an alternative statement
(such ass `err == nil`), it must be documented. The body of the if statement
should be at most 4 lines, but usually only one. Anything requiring more lines
needs to be its own function.

Example:

```go
block, err := s.AcceptBlock()
if err != nil {
	handleAcceptBlockErr(block, err)
	return
}
```

Sanity Checks
-------------

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

If the code is continually checking items that should be universally true,
mistakes are easier to catch during testing, and side effects are less likely
to go unnoticed.
