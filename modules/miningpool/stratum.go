package pool

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

// StratumMsg contains stratum messages over TCP
type StratumRequestMsg struct {
	ID     int           `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

type StratumResponseMsg struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// Handler represents the status (open/closed) of each connection
type Handler struct {
	conn   net.Conn
	closed chan bool
	p      *Pool
}

// Listen listens on a connection for incoming data and acts on it
func (h *Handler) Listen() { // listen connection for incomming data
	defer h.conn.Close()
	h.p.log.Println("New connection from " + h.conn.RemoteAddr().String())
	// bf := bufio.NewReader(h.conn)
	dec := json.NewDecoder(h.conn)
	for {
		var m StratumRequestMsg
		err := dec.Decode(&m)
		// line, err := bf.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				h.p.log.Println("End connection")
			}
			h.closed <- true // send to dispatcher, that connection is closed
			return
		}

		switch m.Method {
		case "mining.subscribe":
			h.handleStratumSubscribe(m)
		default:
			h.p.log.Debugln("Unknown stratum method: ", m.Method)
		}
	}
}

func (h *Handler) sendResponse(r StratumResponseMsg) {
	b, err := json.Marshal(r)
	if err != nil {
		h.p.log.Debugln("json marshal failed for id: ", r.ID, err)
	} else {
		_, err = h.conn.Write(b)
		if err != nil {
			h.p.log.Debugln("connection write failed for id: ", r.ID, err)
		}
		newline := []byte{'\n'}
		h.conn.Write(newline)
		h.p.log.Debugln(string(b))
	}
}
func (h *Handler) handleStratumSubscribe(m StratumRequestMsg) {
	h.p.log.Debugln("ID = "+strconv.Itoa(m.ID)+", Method = "+m.Method+", params = ", m.Params)

	r := StratumResponseMsg{ID: m.ID}

	diff := fmt.Sprintf(`"mining.set_difficulty", "%s"`, "b4b6693b72a50c7116db18d6497cac52")
	notify := fmt.Sprintf(`"mining.notify", "%s"`, "ae6812eb4cd7735a302a8a9dd95cf71f")
	extranonse1 := "08000002"
	extranonse2 := 4
	raw := fmt.Sprintf(`[ [ [%s], [%s]], "%s", %d]`, diff, notify, extranonse1, extranonse2)
	r.Result = json.RawMessage(raw)
	r.Error = json.RawMessage(`null`)
	// {"id": 1, "result": [ [ ["mining.set_difficulty", "b4b6693b72a50c7116db18d6497cac52"], ["mining.notify", "ae6812eb4cd7735a302a8a9dd95cf71f"]], "08000002", 4], "error": null}\n
	h.sendResponse(r)
}

// Dispatcher contains a map of ip addresses to handlers
type Dispatcher struct {
	handlers map[string]*Handler `map:"map[ip]*Handler"`
	mu       sync.RWMutex
	p        *Pool
}

//AddHandler connects the incoming connection to the handler which will handle it
func (d *Dispatcher) AddHandler(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	handler := &Handler{conn, make(chan bool, 1), d.p}
	d.mu.Lock()
	d.handlers[addr] = handler
	d.mu.Unlock()

	go handler.Listen()

	<-handler.closed // when connection closed, remove handler from handlers
	d.mu.Lock()
	delete(d.handlers, addr)
	d.mu.Unlock()
}

// ListenHandlers listens on a passed port and upon accepting the incoming connection, adds the handler to deal with it
func (d *Dispatcher) ListenHandlers(port string) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Println(err)
		return
	}

	defer ln.Close()

	for {
		conn, err := ln.Accept() // accept connection
		if err != nil {
			log.Println(err)
			continue
		}

		tcpconn := conn.(*net.TCPConn)
		tcpconn.SetKeepAlive(true)
		tcpconn.SetKeepAlivePeriod(10 * time.Second)

		go d.AddHandler(conn)
	}
}

// func main() {
//     dispatcher := &Dispatcher{make(map[string]*Handler)}
//     dispatcher.ListenHandlers(3000)
// }
