package chat

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const defaultHistorySize = 50

type Message struct {
	Type string    `json:"type"`
	Room string    `json:"room"`
	User string    `json:"user"`
	Text string    `json:"text,omitempty"`
	At   time.Time `json:"at"`
}

type Room struct {
	connections map[*websocket.Conn]string
	history     []Message
}

type Hub struct {
	mu          sync.Mutex
	rooms       map[string]*Room
	historySize int
}

func NewHub(historySize int) *Hub {
	if historySize <= 0 {
		historySize = defaultHistorySize
	}
	return &Hub{
		rooms:       make(map[string]*Room),
		historySize: historySize,
	}
}

func (h *Hub) Join(room string, ws *websocket.Conn, user string) []Message {
	var history []Message
	h.mu.Lock()
	r := h.roomLocked(room)
	r.connections[ws] = user
	history = append(history, r.history...)
	h.mu.Unlock()

	h.Broadcast(Message{
		Type: "user_join",
		Room: room,
		User: user,
		At:   time.Now().UTC(),
	})

	return history
}

func (h *Hub) Leave(room string, ws *websocket.Conn) {
	var user string
	h.mu.Lock()
	if r, ok := h.rooms[room]; ok {
		if u, exists := r.connections[ws]; exists {
			user = u
		}
		delete(r.connections, ws)
	}
	h.mu.Unlock()

	_ = ws.Close()

	if user != "" {
		h.Broadcast(Message{
			Type: "user_leave",
			Room: room,
			User: user,
			At:   time.Now().UTC(),
		})
	}
}

func (h *Hub) Broadcast(msg Message) {
	if msg.At.IsZero() {
		msg.At = time.Now().UTC()
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[msg.Room]
	if !ok {
		return
	}

	if msg.Type == "message" {
		r.history = append(r.history, msg)
		if len(r.history) > h.historySize {
			r.history = r.history[len(r.history)-h.historySize:]
		}
	}

	for ws := range r.connections {
		if err := ws.WriteMessage(websocket.TextMessage, payload); err != nil {
			_ = ws.Close()
			delete(r.connections, ws)
		}
	}
}

func (h *Hub) History(room string) []Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[room]; ok {
		return append([]Message(nil), r.history...)
	}
	return nil
}

func (h *Hub) User(room string, ws *websocket.Conn) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r, ok := h.rooms[room]; ok {
		return r.connections[ws]
	}
	return ""
}

func (h *Hub) roomLocked(room string) *Room {
	r, ok := h.rooms[room]
	if !ok {
		r = &Room{connections: make(map[*websocket.Conn]string)}
		h.rooms[room] = r
	}
	return r
}
