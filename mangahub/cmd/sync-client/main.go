package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

type AnyEvent map[string]any

func main() {
	addr := flag.String("addr", "127.0.0.1:7070", "TCP sync server address")
	pretty := flag.Bool("pretty", true, "pretty print JSON events")
	flag.Parse()

	for {
		if err := run(*addr, *pretty); err != nil {
			log.Printf("[sync-client] disconnected: %v", err)
		}
		time.Sleep(1 * time.Second) // auto reconnect
	}
}

func run(addr string, pretty bool) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	log.Printf("[sync-client] connected to %s", addr)

	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := sc.Bytes()

		if !pretty {
			fmt.Println(string(line))
			continue
		}

		var obj AnyEvent
		if err := json.Unmarshal(line, &obj); err != nil {
			// not JSON? print raw
			fmt.Println(string(line))
			continue
		}

		b, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(b))
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return os.ErrClosed
}
