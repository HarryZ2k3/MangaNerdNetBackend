package notify

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"sync"
)

const (
	RegisterMessageType   = "register"
	NewChapterMessageType = "new_chapter"
)

type RegisterMessage struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
}

type NewChapterMessage struct {
	Type    string `json:"type"`
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
}

type Client struct {
	UserID string
	Addr   *net.UDPAddr
}

type Registry struct {
	mu      sync.RWMutex
	clients map[string]Client
}

func NewRegistry() *Registry {
	return &Registry{clients: make(map[string]Client)}
}

func (r *Registry) Register(userID string, addr *net.UDPAddr) {
	if userID == "" || addr == nil {
		return
	}
	r.mu.Lock()
	r.clients[userID] = Client{UserID: userID, Addr: addr}
	r.mu.Unlock()
}

func (r *Registry) Remove(userID string) {
	r.mu.Lock()
	delete(r.clients, userID)
	r.mu.Unlock()
}

func (r *Registry) Snapshot() []Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clients := make([]Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

type Server struct {
	addr     string
	registry *Registry
	logger   *log.Logger
	conn     *net.UDPConn
}

func NewServer(addr string, registry *Registry, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{addr: addr, registry: registry, logger: logger}
}

func (s *Server) Run() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	s.conn = conn
	defer conn.Close()

	s.logger.Printf("UDP notify server listening on %s", s.addr)

	buffer := make([]byte, 2048)
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		msg, err := parseRegisterMessage(buffer[:n])
		if err != nil {
			s.logger.Printf("invalid UDP message from %s: %v", addr, err)
			continue
		}
		if msg.Type != RegisterMessageType {
			continue
		}
		s.registry.Register(msg.UserID, addr)
		s.logger.Printf("registered UDP client %s (%s)", msg.UserID, addr)
	}
}

func (s *Server) BroadcastNewChapter(mangaID string, chapter int) {
	if s.conn == nil {
		s.logger.Printf("UDP notify server not running")
		return
	}
	payload, err := json.Marshal(NewChapterMessage{
		Type:    NewChapterMessageType,
		MangaID: mangaID,
		Chapter: chapter,
	})
	if err != nil {
		s.logger.Printf("failed to marshal broadcast: %v", err)
		return
	}

	clients := s.registry.Snapshot()
	for _, client := range clients {
		s.sendWithRetry(client, payload)
	}
}

func (s *Server) sendWithRetry(client Client, payload []byte) {
	if err := s.sendOnce(client, payload); err == nil {
		return
	}
	if err := s.sendOnce(client, payload); err != nil {
		s.logger.Printf("failed to notify user %s at %s: %v", client.UserID, client.Addr, err)
		s.registry.Remove(client.UserID)
	}
}

func (s *Server) sendOnce(client Client, payload []byte) error {
	if client.Addr == nil {
		return errors.New("missing client address")
	}
	_, err := s.conn.WriteToUDP(payload, client.Addr)
	return err
}

func parseRegisterMessage(data []byte) (RegisterMessage, error) {
	var msg RegisterMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return msg, err
	}
	if msg.UserID == "" || msg.Type == "" {
		return msg, errors.New("missing required fields")
	}
	return msg, nil
}
