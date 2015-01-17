package network

import (
	"errors"
	"net"
	"reflect"
	"time"

	"github.com/NebulousLabs/Sia/encoding"
)

// reflection helpers
var (
	typeOfError = reflect.TypeOf((*error)(nil)).Elem()
	typeOfConn  = reflect.TypeOf((*net.Conn)(nil)).Elem()
)

// handlerName truncates a string to 8 bytes. If len(name) < 8, the remaining
// bytes are 0. A handlerName is specified at the beginning of each network
// call, indicating which function should handle the connection.
func handlerName(name string) []byte {
	b := make([]byte, 8)
	copy(b, name)
	return b
}

// Call establishes a TCP connection to the Address, calls the provided
// function on it, and closes the connection.
func (na Address) Call(name string, fn func(net.Conn) error) error {
	conn, err := net.DialTimeout("tcp", string(na), timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	// set default deadline
	// note: fn can extend this deadline as needed
	conn.SetDeadline(time.Now().Add(timeout))
	// write header
	if _, err := conn.Write(handlerName(name)); err != nil {
		return err
	}
	return fn(conn)
}

// RPC performs a Remote Procedure Call by sending the procedure name and
// encoded argument, and decoding the response into the supplied object.
// 'resp' must be a pointer. If arg is nil, no object is sent. If 'resp' is
// nil, no response is read.
func (na *Address) RPC(name string, arg, resp interface{}) error {
	return na.Call(name, func(conn net.Conn) error {
		// write arg
		if arg != nil {
			if _, err := encoding.WriteObject(conn, arg); err != nil {
				return err
			}
		}
		// read resp
		if resp != nil {
			if err := encoding.ReadObject(conn, resp, maxMsgLen); err != nil {
				return err
			}
		}
		// read err
		var errStr string
		if err := encoding.ReadObject(conn, &errStr, maxMsgLen); err != nil {
			return err
		} else if errStr != "" {
			return errors.New(errStr)
		}
		return nil
	})
}

// Broadcast calls the RPC on each peer in the address book.
func (tcps *TCPServer) Broadcast(name string, arg, resp interface{}) {
	for _, addr := range tcps.AddressBook() {
		// TODO: remove unresponsive peers
		_ = addr.RPC(name, arg, resp)
	}
}

// RegisterRPC registers a function as an RPC handler for a given identifier.
// The function must be one of four possible types:
//     func(net.Conn) error
//     func(Type) (Type, error)
//     func(Type) error
//     func() (Type, error)
// To call an RPC, use Address.RPC, supplying the same identifier given to
// RegisterRPC. Identifiers should always use PascalCase.
func (tcps *TCPServer) RegisterRPC(name string, fn interface{}) error {
	// all handlers are functions with 0 or 1 ins and 1 or 2 outs, the last of
	// which must be an error.
	val, typ := reflect.ValueOf(fn), reflect.TypeOf(fn)
	if typ.Kind() != reflect.Func || typ.NumIn() > 1 || typ.NumOut() > 2 ||
		typ.NumOut() < 1 || typ.Out(typ.NumOut()-1) != typeOfError {
		panic("registered function has wrong type signature")
	}

	var handler func(net.Conn) error
	switch {
	// func(net.Conn) error
	case typ.NumIn() == 1 && typ.NumOut() == 1 && typ.In(0) == typeOfConn:
		handler = fn.(func(net.Conn) error)
	// func(Type) (Type, error)
	case typ.NumIn() == 1 && typ.NumOut() == 2:
		handler = registerRPC(val, typ)
	// func(Type) error
	case typ.NumIn() == 1 && typ.NumOut() == 1:
		handler = registerArg(val, typ)
	// func() (Type, error)
	case typ.NumIn() == 0 && typ.NumOut() == 2:
		handler = registerResp(val, typ)

	default:
		panic("registered function has wrong type signature")
	}

	ident := string(handlerName(name))
	tcps.Lock()
	tcps.handlerMap[ident] = handler
	tcps.Unlock()

	return nil
}

// registerRPC is for handlers that take an argument return a value. The input
// is decoded and passed to fn, whose return value is written back to the
// caller. fn must have the type signature:
//   func(Type, *Type) error
func registerRPC(fn reflect.Value, typ reflect.Type) func(net.Conn) error {
	return func(conn net.Conn) error {
		// read arg
		arg := reflect.New(typ.In(0))
		if err := encoding.ReadObject(conn, arg.Interface(), maxMsgLen); err != nil {
			return err
		}
		// call fn
		retvals := fn.Call([]reflect.Value{arg.Elem()})
		resp, errInter := retvals[0].Interface(), retvals[1].Interface()
		// write resp
		if _, err := encoding.WriteObject(conn, resp); err != nil {
			return err
		}
		// write err
		var errStr string
		if errInter != nil {
			errStr = errInter.(error).Error()
		}
		_, err := encoding.WriteObject(conn, errStr)
		return err
	}
}

// registerArg is for RPCs that do not return a value.
func registerArg(fn reflect.Value, typ reflect.Type) func(net.Conn) error {
	return func(conn net.Conn) error {
		// read arg
		arg := reflect.New(typ.In(0))
		if err := encoding.ReadObject(conn, arg.Interface(), maxMsgLen); err != nil {
			return err
		}
		// call fn on object
		errInter := fn.Call([]reflect.Value{arg.Elem()})[0].Interface()
		// write err
		var errStr string
		if errInter != nil {
			errStr = errInter.(error).Error()
		}
		_, err := encoding.WriteObject(conn, errStr)
		return err
	}
}

// registerResp is for RPCs that do not take a value.
func registerResp(fn reflect.Value, typ reflect.Type) func(net.Conn) error {
	return func(conn net.Conn) error {
		// call fn
		retvals := fn.Call(nil)
		resp, errInter := retvals[0].Interface(), retvals[1].Interface()
		// write resp
		if _, err := encoding.WriteObject(conn, resp); err != nil {
			return err
		}
		// write err
		var errStr string
		if errInter != nil {
			errStr = errInter.(error).Error()
		}
		_, err := encoding.WriteObject(conn, errStr)
		return err
	}
}
