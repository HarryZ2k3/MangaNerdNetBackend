package sync

import (
	"bufio"
	"log"
	"net"
)

type Server struct {
	Addr string
	Hub  *Hub
}

func NewServer(addr string, hub *Hub) *Server {
	return &Server{Addr: addr, Hub: hub}
}

func (s *Server) Run() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	log.Printf("[tcp-sync] listening on %s", s.Addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		s.Hub.Add(conn)
		s.Hub.Welcome(conn)
		log.Printf("[tcp-sync] client connected: %s", conn.RemoteAddr())

		go func(c net.Conn) {
			defer func() {
				s.Hub.Remove(c)
				log.Printf("[tcp-sync] client disconnected: %s", c.RemoteAddr())
			}()

			// Keep the connection alive; if client sends anything, just consume.
			sc := bufio.NewScanner(c)
			for sc.Scan() {
				// ignore incoming lines
			}
		}(conn)
	}
}
