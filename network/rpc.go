package network

import (
	"errors"
	"net"
	"reflect"

	"github.com/NebulousLabs/Andromeda/encoding"
)

func ReadPrefix(conn net.Conn) ([]byte, error) {
	prefix := make([]byte, 4)
	if n, err := conn.Read(prefix); err != nil || n != len(prefix) {
		return nil, errors.New("could not read length prefix")
	}
	msgLen := int(encoding.DecUint64(prefix))
	if msgLen > maxMsgLen {
		return nil, errors.New("message too long")
	}
	// read msgLen bytes
	var data []byte
	buf := make([]byte, 1024)
	for total := 0; total < msgLen; {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		data = append(data, buf[:n]...)
		total += n
	}
	if len(data) != msgLen {
		return nil, errors.New("message length mismatch")
	}
	return data, nil
}

func ReadObject(conn net.Conn, obj interface{}) error {
	data, err := ReadPrefix(conn)
	if err != nil {
		return err
	}
	return encoding.Unmarshal(data, obj)
}

func WritePrefix(conn net.Conn, data []byte) (int, error) {
	encLen := encoding.EncUint64(uint64(len(data)))
	return conn.Write(append(encLen[:4], data...))
}

func WriteObject(conn net.Conn, obj interface{}) (int, error) {
	return WritePrefix(conn, encoding.Marshal(obj))
}

// RPC performs a Remote Procedure Call by sending the procedure name and
// encoded argument, and decoding the response into the supplied object.
// 'resp' must be a pointer. If arg is nil, no object is sent. If 'resp' is
// nil, no response is read.
func (na *NetAddress) RPC(t byte, arg, resp interface{}) error {
	return na.Call(func(conn net.Conn) error {
		conn.Write([]byte{t})
		var data []byte
		if arg != nil {
			data = encoding.Marshal(arg)
		}
		if _, err := WritePrefix(conn, data); err != nil {
			return err
		}
		if resp != nil {
			return ReadObject(conn, resp)
		}
		return nil
	})
}

func (tcps *TCPServer) Register(t byte, fn interface{}) {
	// all handlers are function with at least one in and one error out
	val, typ := reflect.ValueOf(fn), reflect.TypeOf(fn)
	if typ.Kind() != reflect.Func || typ.NumIn() < 1 ||
		typ.NumOut() != 1 || typ.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		panic("registered function has wrong type signature")
	}

	switch {
	// func(net.Conn, []byte) error
	case typ.NumIn() == 2 && typ.In(0) == reflect.TypeOf((*net.Conn)(nil)).Elem() && typ.In(1) == reflect.TypeOf([]byte{}):
		tcps.handlerMap[t] = fn.(func(net.Conn, []byte) error)
	// func(Type, *Type) error
	case typ.NumIn() == 2 && typ.In(0).Kind() != reflect.Ptr && typ.In(1).Kind() == reflect.Ptr:
		tcps.registerRPC(t, val, typ)
	// func(Type) error
	case typ.NumIn() == 1 && typ.In(0).Kind() != reflect.Ptr:
		tcps.registerArg(t, val, typ)
	// func(*Type) error
	case typ.NumIn() == 1 && typ.In(0).Kind() == reflect.Ptr:
		tcps.registerResp(t, val, typ)
	default:
		panic("registered function has wrong type signature")
	}
}

// registerRPC is for handlers that return a value. The input is decoded and
// passed to fn, which stores its result in a pointer argument. This argument
// is then written back to the caller. fn must have the type signature:
//     func(Type, *Type) error
func (tcps *TCPServer) registerRPC(t byte, fn reflect.Value, typ reflect.Type) {
	tcps.handlerMap[t] = func(conn net.Conn, b []byte) error {
		// create object to decode into
		arg := reflect.New(typ.In(0))
		if err := encoding.Unmarshal(b, arg.Interface()); err != nil {
			return err
		}
		// call fn on object
		resp := reflect.New(typ.In(1))
		if err := fn.Call([]reflect.Value{arg.Elem(), resp})[0].Interface(); err != nil {
			return err.(error)
		}
		// write response
		_, err := WriteObject(conn, resp.Elem().Interface())
		return err
	}
}

// registerArg is for RPCs that do not return a value.
func (tcps *TCPServer) registerArg(t byte, fn reflect.Value, typ reflect.Type) {
	tcps.handlerMap[t] = func(_ net.Conn, b []byte) error {
		// create object to decode into
		arg := reflect.New(typ.In(0))
		if err := encoding.Unmarshal(b, arg.Interface()); err != nil {
			return err
		}
		// call fn on object
		if err := fn.Call([]reflect.Value{arg.Elem()})[0].Interface(); err != nil {
			return err.(error)
		}
		return nil
	}
}

// registerResp is for RPCs that do not take a value.
func (tcps *TCPServer) registerResp(t byte, fn reflect.Value, typ reflect.Type) {
	tcps.handlerMap[t] = func(conn net.Conn, _ []byte) error {
		// create object to hold response
		resp := reflect.New(typ.In(0))
		// call fn
		if err := fn.Call([]reflect.Value{resp})[0].Interface(); err != nil {
			return err.(error)
		}
		// write response
		_, err := WriteObject(conn, resp.Elem().Interface())
		return err
	}
}

// sendHostname replies to the send with the sender's external IP.
func sendHostname(conn net.Conn, _ []byte) error {
	host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	_, err := WritePrefix(conn, []byte(host))
	return err
}

// sharePeers transmits at most 'num' peers over the connection.
// TODO: choose random peers?
func (tcps *TCPServer) sharePeers(addrs *[]NetAddress) error {
	for addr := range tcps.addressbook {
		if len(*addrs) == 10 { // arbitrary
			break
		}
		*addrs = append(*addrs, addr)
	}
	return nil
}

// addPeer adds the connecting peer to its address book
func (tcps *TCPServer) addPeer(peer NetAddress) error {
	tcps.addressbook[peer] = struct{}{}
	return nil
}
