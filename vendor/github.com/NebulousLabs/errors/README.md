errors
------

`errors` is an extension of and drop in replacement for the standard library
errors package. Multiple errors can be composed into a single error with the
`Compose` function, or a single error can be extended by adding context using
the `Extend` function. The result can be checked for the presense of a specific
error using `Contains`. If any of the underlying composed or extended errors or
extensions match, `Contains` will return true.

This package is especially beneficial during testing, where you may wish to
check for presense of a specific error, but that specific error may be extending
another error or may have been extended by another function somewhere else in
the call stack.

Example:

```go
var errOne = errors.New("one")
var errTwo = errors.New("two")
_, errDNE = os.Open("file.txt")

extended := errors.Extend(errOne, errDNE)    // "[one; open file.txt: no such file or directory]"
extended2 := errors.Extend(errTwo, extended) // "[two; one; open file.txt: no such file or directory]"
errors.IsOSNotExist(extended)                // true
errors.Contains(extended, errDNE)            // true
errors.Contains(extended, errOne)            // true
errors.Contains(extended, errTwo)            // false
errors.Contains(extended2, errTwo)           // true
```

The `Compose` function works similarly to extend, however instead of simply
combining the two errors into a joined set, a new Error is created that contains
both underlying errors individually. The `Contains` and `IsOSNotExist` functions
will still work as expected, returning true if they apply to any of the nested
underlying errors.

```go
var errOne = errors.New("one")
var errTwo = errors.New("two")
composed := errors.Compose(errOne, errTwo)                         // "[[one]; [two]]"
composed2 := errors.Compose(errOne, errors.Extend(errTwo, errTwo)) // "[[one]; [two; two]]"
errors.Contains(composed2, errTwo) // true
```
