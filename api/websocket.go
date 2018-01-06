package api

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	//WriteTimeout max time the socket writer will wait
	WriteTimeout = 5 * time.Second
)

//WebsocketHub encapsulates the Sia websocket implementation
type WebsocketHub struct {
	//*Subscribers map contains the current list of block update subscribers and transaction subscribers
	blockSubscribers map[*Subscriber]bool
	txSubscribers    map[*Subscriber]bool
	//push to broadcastBlock chan to broadcast a new transaction to all block update subscribers
	broadcastTx chan []byte
	//push to broadcastBlock chan to broadcast a block to all block update subscribers
	broadcastBlock chan []byte
	//push to registerTx to register a new transaction subscriber
	registerTx chan *Subscriber
	//push to registerTx to unregister a new transaction subscriber
	unregisterTx chan *Subscriber
	//push to registerTx to register a new block update subscriber
	registerBlock chan *Subscriber
	//push to registerTx to unregister a new block update subscriber
	unregisterBlock chan *Subscriber
}

//Subscriber is an encapsulation of a single connection to the websocket hub
type Subscriber struct {
	hub  *WebsocketHub
	conn *websocket.Conn
	send chan []byte
}

//Upgrader upgrades HTTP connections to WS
var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

//SocketWriter should be called in a goroutine.  SocketWriter manages writing to the subscriber connection via channel buffers
func (s *Subscriber) SocketWriter() {
	defer func() {
		if s.conn != nil {
			s.conn.Close()
		}
	}()
	for {
		select {
		case message, ok := <-s.send:
			{
				s.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
				if !ok {
					s.conn.WriteMessage(websocket.CloseMessage, []byte{})
					//Closed in the defer
					return
				}
				w, err := s.conn.NextWriter(websocket.TextMessage)
				if err != nil {
					//Closed in the defer
					log.Printf("Error reading from connection: %s", err)
					return
				}
				w.Write(message)
				n := len(s.send)
				for i := 0; i < n; i++ {
					w.Write(<-s.send)
				}
				if err := w.Close(); err != nil {
					//Closed in the defer
					log.Printf("Error writing connection: %s", err)
					return
				}
			}
		}
	}
}

//StartWebsocketHub should be started in a goroutine.  This handles brodcasting to subscribers via chan
func (h *WebsocketHub) StartWebsocketHub() {
	for {
		select {
		case blockSubscriber := <-h.registerBlock:
			h.blockSubscribers[blockSubscriber] = true
		case blockSubscriber := <-h.unregisterBlock:
			if _, ok := h.blockSubscribers[blockSubscriber]; ok {
				delete(h.blockSubscribers, blockSubscriber)
				close(blockSubscriber.send)
			}
		case txSubscriber := <-h.registerTx:
			h.txSubscribers[txSubscriber] = true
		case txSubscriber := <-h.unregisterTx:
			if _, ok := h.txSubscribers[txSubscriber]; ok {
				delete(h.txSubscribers, txSubscriber)
				close(txSubscriber.send)
			}
		case block := <-h.broadcastBlock:
			for subscriber := range h.blockSubscribers {
				select {
				case subscriber.send <- block:
				default:
					close(subscriber.send)
					delete(h.blockSubscribers, subscriber)
				}
			}
		case tx := <-h.broadcastTx:
			for subscriber := range h.txSubscribers {
				select {
				case subscriber.send <- tx:
				default:
					close(subscriber.send)
					delete(h.txSubscribers, subscriber)
				}
			}
		}
	}
}
