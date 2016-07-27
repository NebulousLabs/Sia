package host

// errors.go is responsible for logging the various errors that the host runs
// into related to operations that cannot immediately provide feedback to the
// user. (e.g. network failures, disk failures, etc.). Different errors should
// be handled and logged differently, depending on severity and frequency, such
// that the person reading the logs is able to see all of the major issues
// without having them obstructed by the minor ones.

import (
	"errors"
	"strings"
	"sync/atomic"
)

type (
	// ErrorCommunication errors are meant to be returned if the host and the
	// renter seem to be miscommunicating. For example, if the renter attempts
	// to pay an insufficient price, there has been a communication error.
	ErrorCommunication string

	// ErrorConnection is meant to be used on errors where the network is
	// returning unexpected errors. For example, sudden disconnects or
	// connection write failures.
	ErrorConnection string

	// ErrorConsensus errors are meant to be used when there are problems
	// related to consensus, such as an inability to submit a storage proof to
	// the blockchain, or an inability to get a file contract revision on to
	// the blockchain.
	ErrorConsensus string

	// ErrorInternal errors are meant to be used if an internal process in the
	// host is malfunctioning, for example if the disk is failing.
	ErrorInternal string
)

// composeErrors will take multiple errors and compose them into a single
// errors with a longer message. Any nil errors used as inputs will be stripped
// out, and if there are zero non-nil inputs then 'nil' will be returned.
//
// The original types of the errors is not preserved at all.
func composeErrors(errs ...error) error {
	// Strip out any nil errors.
	var errStrings []string
	for _, err := range errs {
		if err != nil {
			errStrings = append(errStrings, err.Error())
		}
	}

	// Return nil if there are no non-nil errors in the input.
	if len(errStrings) <= 0 {
		return nil
	}

	// Combine all of the non-nil errors into one larger return value.
	return errors.New(strings.Join(errStrings, "; "))
}

// extendErr will return an error that is the same type as the input error, but
// prefixed with the provided context. This only works for the error types
// defined in the host package. If the input error is nil, the extension is
// ignored and nil will be returned.
func extendErr(s string, err error) error {
	if err == nil {
		return nil
	}

	switch v := err.(type) {
	case ErrorCommunication:
		return ErrorCommunication(s) + v
	case ErrorConnection:
		return ErrorConnection(s) + v
	case ErrorConsensus:
		return ErrorConsensus(s) + v
	case ErrorInternal:
		return ErrorInternal(s) + v
	default:
		return errors.New(s + err.Error())
	}

}

// Error satisfies the Error interface for the ErrorCommunication type.
func (ec ErrorCommunication) Error() string {
	return "communication error: " + string(ec)
}

// Error satisfies the Error interface for the ErrorConnection type.
func (ec ErrorConnection) Error() string {
	return "connection error: " + string(ec)
}

// Error satisfies the Error interface for the ErrorConsensus type.
func (ec ErrorConsensus) Error() string {
	return "consensus error: " + string(ec)
}

// Error satisfies the Error interface for the ErrorInternal type.
func (ec ErrorInternal) Error() string {
	return "internal error: " + string(ec)
}

// mangedLogError will take an error and log it to the host, depending on the
// type of error and whether or not the DEBUG flag has been set.
func (h *Host) managedLogError(err error) {
	// Determine the error type and the number of errors we've seen of that
	// type previously.
	var num uint64
	switch err.(type) {
	case ErrorCommunication:
		atomic.AddUint64(&h.atomicCommunicationErrors, 1)
		num = atomic.LoadUint64(&h.atomicCommunicationErrors)
	case ErrorConnection:
		atomic.AddUint64(&h.atomicConnectionErrors, 1)
		num = atomic.LoadUint64(&h.atomicConnectionErrors)
	case ErrorConsensus:
		atomic.AddUint64(&h.atomicConsensusErrors, 1)
		num = atomic.LoadUint64(&h.atomicConsensusErrors)
	case ErrorInternal:
		atomic.AddUint64(&h.atomicInternalErrors, 1)
		num = atomic.LoadUint64(&h.atomicInternalErrors)
	default:
		atomic.AddUint64(&h.atomicNormalErrors, 1)
		num = atomic.LoadUint64(&h.atomicNormalErrors)
	}

	// If we've seen less than 250 of that type of error before, log the error
	// as a normal logging statement. If we've seen more than 250 of that error
	// before, log the error as a debugging statement.
	if num < 250 {
		h.log.Println(err)
	} else {
		h.log.Debugln(err)
	}
}
