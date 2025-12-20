package sync

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

type Hub struct {
	mu      sync.Mutex
	clients map[net.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[net.Conn]struct{}),
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

func (h *Hub) BroadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	// newline-delimited JSON (NDJSON) so clients can read line-by-line
	b = append(b, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()

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
}

func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *Hub) Welcome(conn net.Conn) {
	msg := fmt.Sprintf("{\"type\":\"welcome\",\"message\":\"connected\",\"clients\":%d}\n", h.Count())
	_, _ = conn.Write([]byte(msg))
}
