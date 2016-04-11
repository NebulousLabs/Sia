package host

import (
	"net"

	"github.com/NebulousLabs/Sia/encoding"
)

// rejectNegotiation will write a rejection response to the connection and
// return the input error composed with the error received from writing to the
// connection.
func rejectNegotiation(conn net.Conn, err error) error {
	writeErr := encoding.WriteObject(conn, err.Error())
	return composeErrors(err, writeErr)
}
