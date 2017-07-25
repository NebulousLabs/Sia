package pool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/NebulousLabs/Sia/crypto"
)

// StratumMsg contains stratum messages over TCP
type StratumRequestMsg struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
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
	err := h.p.tg.Add()
	if err != nil {
		// If this goroutine is not run before shutdown starts, this
		// codeblock is reachable.
		return
	}
	defer h.p.tg.Done()

	h.p.log.Println("New connection from " + h.conn.RemoteAddr().String())
	dec := json.NewDecoder(h.conn)
	for {
		var m StratumRequestMsg
		select {
		case <-h.p.tg.StopChan():
			h.closed <- true // not closed until we return but we signal now so our parent knows
			return
		default:
			err := dec.Decode(&m)
			if err != nil {
				if err == io.EOF {
					h.p.log.Println("End connection")
				}
				h.closed <- true // send to dispatcher, that connection is closed
				return
			}
		}
		switch m.Method {
		case "mining.subscribe":
			h.handleStratumSubscribe(m)
		case "mining.authorize":
			h.handleStatumAuthorize(m)
			h.sendStratumNotify()
		case "mining.extranonce.subscribe":
			h.handleStratumNonceSubscribe(m)
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
func (h *Handler) sendRequest(r StratumRequestMsg) {
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

// handleStratumSubscribe message is the first message received and allows the pool to tell the miner
// the difficulty as well as notify, extranonse1 and extranonse2
//
// TODO: Pull the appropriate data from either in memory or persistent store as required
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

// handleStratumAuthorize allows the pool to tie the miner connection to a particular user or wallet
//
// TODO: THis has to tie to either a connection specific record, or relate to a backend user, worker, password store
func (h *Handler) handleStatumAuthorize(m StratumRequestMsg) {
	h.p.log.Debugln("ID = "+strconv.Itoa(m.ID)+", Method = "+m.Method+", params = ", m.Params)

	r := StratumResponseMsg{ID: m.ID}
	r.Result = json.RawMessage(`true`)
	r.Error = json.RawMessage(`null`)

	h.sendResponse(r)
}

// handleStratumExtranonceSubscribe tells the pool that this client can handle the extranonce info
//
// TODO: Not sure we have to anything if all our clients support this.
func (h *Handler) handleStratumNonceSubscribe(m StratumRequestMsg) {
	h.p.log.Debugln("ID = "+strconv.Itoa(m.ID)+", Method = "+m.Method+", params = ", m.Params)

	r := StratumResponseMsg{ID: m.ID}
	r.Result = json.RawMessage(`true`)
	r.Error = json.RawMessage(`null`)

	h.sendResponse(r)

}

func (h *Handler) sendStratumNotify() {
	var r StratumRequestMsg
	r.Method = "mining.notify"
	r.ID = 1 // assuming this ID is the response to the original subscribe which appears to be a 1
	bh, target, err := h.p.HeaderForWork()
	if err != nil {
		h.p.log.Println("Error getting header for work: ", err)
		return
	}
	h.p.log.Println("BH->ParentID: ", bh.ParentID)
	h.p.log.Println("BH->Nonce: ", bh.Nonce)
	h.p.log.Println("BH->Timestamp: ", bh.Timestamp)
	h.p.log.Println("BH->MerkleRoot: ", bh.MerkleRoot)
	h.p.log.Println("Target: ", target)
	fmt.Println(target)
	branch1 := crypto.NewTree()
	var buf bytes.Buffer
	for _, payout := range h.p.sourceBlock.MinerPayouts {
		payout.MarshalSia(&buf)
		branch1.Push(buf.Bytes())
		buf.Reset()
	}
	branch2 := crypto.NewTree()

	for _, txn := range h.p.sourceBlock.Transactions {
		txn.MarshalSia(&buf)
		branch2.Push(buf.Bytes())
		buf.Reset()
	}
	merkleBranch := fmt.Sprintf(`%s", "%s`, branch1.Root().String(), branch2.Root().String())
	jobid := "bf"
	prevhash := bh.ParentID
	//"000000000000052714f51ebea73d6310db96d54a8399c5802e42508ea2486717"
	coinb1 := "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000010000000000000020000000000000004e6f6e536961000000000000000000000000000000000000"
	coinb2 := "0000000000000000"
	//	merkleBranch := `356464cda3f7a83a350aeb3ae5101ff56799cd68ad48b475426141540876bd31", "9cb176ec5b06898ef40f0e73242e0b0ff9d34ece67a241d529f2c18c67c73803`
	version := ""
	nbits := "1a08645a"
	ntime := fmt.Sprintf("%016x", bh.Timestamp<<32)
	//"58258e5700000000"
	cleanJobs := false
	raw := fmt.Sprintf(`[ "%s", "%s", "%s", "%s", ["%s"], "%s", "%s", "%s", %t ]`,
		jobid, prevhash, coinb1, coinb2, merkleBranch, version, nbits, ntime, cleanJobs)
	r.Params = json.RawMessage(raw)
	// {"params": ["bf", "4d16b6f85af6e2198f44ae2a6de67f78487ae5611b77c6c0440b921e00000000",
	//"01000000010000000000000000000000000000000000000000000000000000000000000000ffffffff20020862062f503253482f04b8864e5008",
	//"072f736c7573682f000000000100f2052a010000001976a914d23fcdf86f7e756a64a7a9688ef9903327048ed988ac00000000", [],
	//"00000002", "1c2ac4af", "504e86b9", false], "id": null, "method": "mining.notify"}
	h.sendRequest(r)
}

// Dispatcher contains a map of ip addresses to handlers
type Dispatcher struct {
	handlers map[string]*Handler `map:"map[ip]*Handler"`
	ln       net.Listener
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

	handler.Listen()

	<-handler.closed // when connection closed, remove handler from handlers
	d.mu.Lock()
	delete(d.handlers, addr)
	d.mu.Unlock()
}

// ListenHandlers listens on a passed port and upon accepting the incoming connection, adds the handler to deal with it
func (d *Dispatcher) ListenHandlers(port string) {
	var err error
	d.ln, err = net.Listen("tcp", ":"+port)
	if err != nil {
		log.Println(err)
		return
	}

	defer d.ln.Close()
	err = d.p.tg.Add()
	if err != nil {
		// If this goroutine is not run before shutdown starts, this
		// codeblock is reachable.
		return
	}
	defer d.p.tg.Done()

	for {
		var conn net.Conn
		var err error
		select {
		case <-d.p.tg.StopChan():
			d.ln.Close()
			return
		default:
			conn, err = d.ln.Accept() // accept connection
			if err != nil {
				//				log.Println(err)
				continue
			}
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
