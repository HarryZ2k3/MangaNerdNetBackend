package sync

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu        sync.Mutex
	clients   map[net.Conn]struct{}
	wsClients map[*websocket.Conn]struct{}
}

type Stats struct {
	TCPClients int `json:"tcp_clients"`
	WSClients  int `json:"ws_clients"`
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[net.Conn]struct{}),
		wsClients: make(map[*websocket.Conn]struct{}),
	}
}

func (h *Hub) Add(conn net.Conn) {
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) Remove(conn net.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
	_ = conn.Close()
}

func (h *Hub) AddWS(ws *websocket.Conn) {
	h.mu.Lock()
	h.wsClients[ws] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) RemoveWS(ws *websocket.Conn) {
	h.mu.Lock()
	delete(h.wsClients, ws)
	h.mu.Unlock()
	_ = ws.Close()
}

func (h *Hub) BroadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	b = append(b, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()

	// TCP clients
	for c := range h.clients {
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		w := bufio.NewWriter(c)
		if _, err := w.Write(b); err != nil {
			_ = c.Close()
			delete(h.clients, c)
			continue
		}
		if err := w.Flush(); err != nil {
			_ = c.Close()
			delete(h.clients, c)
			continue
		}
	}

	// WebSocket clients
	for ws := range h.wsClients {
		if err := ws.WriteMessage(websocket.TextMessage, b); err != nil {
			_ = ws.Close()
			delete(h.wsClients, ws)
		}
	}
}

func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *Hub) Stats() Stats {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Stats{
		TCPClients: len(h.clients),
		WSClients:  len(h.wsClients),
	}
}

func (h *Hub) Welcome(conn net.Conn) {
	msg := fmt.Sprintf("{\"type\":\"welcome\",\"message\":\"connected\",\"clients\":%d}\n", h.Count())
	_, _ = conn.Write([]byte(msg))
}
