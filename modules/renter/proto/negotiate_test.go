package proto

import (
	"errors"
	"net"
	"testing"

	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/encoding"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/types"
)

// TestNegotiateRevisionStopResponse tests that when the host sends
// StopResponse, the renter continues processing the revision instead of
// immediately terminating.
func TestNegotiateRevisionStopResponse(t *testing.T) {
	// simulate a renter-host connection
	rConn, hConn := net.Pipe()

	// handle the host's half of the pipe
	go func() {
		defer hConn.Close()
		// read revision
		encoding.ReadObject(hConn, new(types.FileContractRevision), 1<<22)
		// write acceptance
		modules.WriteNegotiationAcceptance(hConn)
		// read txn signature
		encoding.ReadObject(hConn, new(types.TransactionSignature), 1<<22)
		// write StopResponse
		modules.WriteNegotiationStop(hConn)
		// write txn signature
		encoding.WriteObject(hConn, types.TransactionSignature{})
	}()

	// since the host wrote StopResponse, we should proceed to validating the
	// transaction. This will return a known error because we are supplying an
	// empty revision.
	_, err := negotiateRevision(rConn, types.FileContractRevision{}, crypto.SecretKey{})
	if err != types.ErrFileContractWindowStartViolation {
		t.Fatalf("expected %q, got \"%v\"", types.ErrFileContractWindowStartViolation, err)
	}
	rConn.Close()

	// same as above, but send an error instead of StopResponse. The error
	// should be returned by negotiateRevision immediately (if it is not, we
	// should expect to see a transaction validation error instead).
	rConn, hConn = net.Pipe()
	go func() {
		defer hConn.Close()
		encoding.ReadObject(hConn, new(types.FileContractRevision), 1<<22)
		modules.WriteNegotiationAcceptance(hConn)
		encoding.ReadObject(hConn, new(types.TransactionSignature), 1<<22)
		// write a sentinel error
		modules.WriteNegotiationRejection(hConn, errors.New("sentinel"))
		encoding.WriteObject(hConn, types.TransactionSignature{})
	}()
	expectedErr := "host did not accept transaction signature: sentinel"
	_, err = negotiateRevision(rConn, types.FileContractRevision{}, crypto.SecretKey{})
	if err == nil || err.Error() != expectedErr {
		t.Fatalf("expected %q, got \"%v\"", expectedErr, err)
	}
	rConn.Close()
}
