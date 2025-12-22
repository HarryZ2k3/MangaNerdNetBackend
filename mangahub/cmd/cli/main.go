package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"mangahub/pkg/models"
)

const defaultBaseURL = "http://localhost:8080"

type tokenData struct {
	Token string `json:"token"`
}

type authResponse struct {
	Token string `json:"token"`
}

type mangaListResponse struct {
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
	Items  []models.MangaDB `json:"items"`
}

func main() {
	global := flag.NewFlagSet("mangahub", flag.ExitOnError)
	baseURL := global.String("api", defaultBaseURL, "API base URL")
	tokenPath := global.String("token", defaultTokenPath(), "token file path")
	if err := global.Parse(os.Args[1:]); err != nil {
		log.Fatalf("parse flags: %v", err)
	}
	args := global.Args()
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	ctx := context.Background()
	cmd := args[0]
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}

	client := &http.Client{Timeout: 15 * time.Second}

	switch cmd {
	case "auth":
		handleAuth(ctx, client, *baseURL, *tokenPath, sub, args[2:])
	case "manga":
		handleManga(ctx, client, *baseURL, sub, args[2:])
	case "library":
		handleLibrary(ctx, client, *baseURL, *tokenPath, sub, args[2:])
	case "progress":
		handleProgress(ctx, client, *baseURL, *tokenPath, sub, args[2:])
	case "sync":
		handleSync(sub, args[2:])
	case "notify":
		handleNotify(*baseURL, sub, args[2:])
	case "chat":
		handleChat(sub, args[2:])
	case "export":
		handleExport(ctx, client, *baseURL, sub, args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func handleAuth(ctx context.Context, client *http.Client, baseURL, tokenPath, sub string, args []string) {
	switch sub {
	case "login":
		fs := flag.NewFlagSet("auth login", flag.ExitOnError)
		email := fs.String("email", "", "email address")
		password := fs.String("password", "", "password")
		_ = fs.Parse(args)

		if *email == "" || *password == "" {
			log.Fatal("email and password are required")
		}

		payload := map[string]string{"email": *email, "password": *password}
		var resp authResponse
		if err := doJSON(ctx, client, http.MethodPost, baseURL+"/auth/login", "", payload, &resp); err != nil {
			log.Fatalf("login failed: %v", err)
		}
		if err := saveToken(tokenPath, resp.Token); err != nil {
			log.Fatalf("save token: %v", err)
		}
		fmt.Println("✅ logged in")
	case "register":
		fs := flag.NewFlagSet("auth register", flag.ExitOnError)
		username := fs.String("username", "", "username")
		email := fs.String("email", "", "email address")
		password := fs.String("password", "", "password")
		_ = fs.Parse(args)

		if *username == "" || *email == "" || *password == "" {
			log.Fatal("username, email, and password are required")
		}

		payload := map[string]string{"username": *username, "email": *email, "password": *password}
		var resp authResponse
		if err := doJSON(ctx, client, http.MethodPost, baseURL+"/auth/register", "", payload, &resp); err != nil {
			log.Fatalf("register failed: %v", err)
		}
		if err := saveToken(tokenPath, resp.Token); err != nil {
			log.Fatalf("save token: %v", err)
		}
		fmt.Println("✅ registered and logged in")
	case "logout":
		if err := clearToken(tokenPath); err != nil {
			log.Fatalf("logout failed: %v", err)
		}
		fmt.Println("✅ logged out")
	default:
		log.Fatal("usage: mangahub auth <login|register|logout>")
	}
}

func handleManga(ctx context.Context, client *http.Client, baseURL, sub string, args []string) {
	switch sub {
	case "search":
		fs := flag.NewFlagSet("manga search", flag.ExitOnError)
		query := fs.String("q", "", "search query")
		status := fs.String("status", "", "status filter")
		genres := fs.String("genres", "", "comma-separated genres")
		limit := fs.Int("limit", 20, "page size")
		offset := fs.Int("offset", 0, "offset")
		_ = fs.Parse(args)

		u, err := url.Parse(baseURL + "/manga")
		if err != nil {
			log.Fatalf("invalid base url: %v", err)
		}
		qv := u.Query()
		if *query != "" {
			qv.Set("q", *query)
		}
		if *status != "" {
			qv.Set("status", *status)
		}
		if *genres != "" {
			qv.Set("genres", *genres)
		}
		qv.Set("limit", fmt.Sprintf("%d", *limit))
		qv.Set("offset", fmt.Sprintf("%d", *offset))
		u.RawQuery = qv.Encode()

		var resp mangaListResponse
		if err := doJSON(ctx, client, http.MethodGet, u.String(), "", nil, &resp); err != nil {
			log.Fatalf("search failed: %v", err)
		}
		printJSON(resp)
	case "show":
		fs := flag.NewFlagSet("manga show", flag.ExitOnError)
		id := fs.String("id", "", "manga id")
		_ = fs.Parse(args)
		if *id == "" {
			log.Fatal("manga id is required")
		}

		var resp models.MangaDB
		if err := doJSON(ctx, client, http.MethodGet, baseURL+"/manga/"+url.PathEscape(*id), "", nil, &resp); err != nil {
			log.Fatalf("show failed: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub manga <search|show>")
	}
}

func handleLibrary(ctx context.Context, client *http.Client, baseURL, tokenPath, sub string, args []string) {
	token := mustToken(tokenPath)
	switch sub {
	case "add":
		fs := flag.NewFlagSet("library add", flag.ExitOnError)
		mangaID := fs.String("manga-id", "", "manga id")
		chapter := fs.Int("chapter", 0, "current chapter")
		status := fs.String("status", "reading", "status")
		_ = fs.Parse(args)
		if *mangaID == "" {
			log.Fatal("manga-id is required")
		}

		payload := map[string]any{
			"manga_id":        *mangaID,
			"current_chapter": *chapter,
			"status":          *status,
		}
		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodPost, baseURL+"/users/library", token, payload, &resp); err != nil {
			log.Fatalf("add failed: %v", err)
		}
		printJSON(resp)
	case "remove":
		fs := flag.NewFlagSet("library remove", flag.ExitOnError)
		mangaID := fs.String("manga-id", "", "manga id")
		_ = fs.Parse(args)
		if *mangaID == "" {
			log.Fatal("manga-id is required")
		}

		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodDelete, baseURL+"/users/library/"+url.PathEscape(*mangaID), token, nil, &resp); err != nil {
			log.Fatalf("remove failed: %v", err)
		}
		printJSON(resp)
	case "list":
		fs := flag.NewFlagSet("library list", flag.ExitOnError)
		status := fs.String("status", "", "status filter")
		limit := fs.Int("limit", 20, "page size")
		offset := fs.Int("offset", 0, "offset")
		_ = fs.Parse(args)

		u, err := url.Parse(baseURL + "/users/library")
		if err != nil {
			log.Fatalf("invalid base url: %v", err)
		}
		qv := u.Query()
		if *status != "" {
			qv.Set("status", *status)
		}
		qv.Set("limit", fmt.Sprintf("%d", *limit))
		qv.Set("offset", fmt.Sprintf("%d", *offset))
		u.RawQuery = qv.Encode()

		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, u.String(), token, nil, &resp); err != nil {
			log.Fatalf("list failed: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub library <add|remove|list>")
	}
}

func handleProgress(ctx context.Context, client *http.Client, baseURL, tokenPath, sub string, args []string) {
	token := mustToken(tokenPath)
	switch sub {
	case "update":
		fs := flag.NewFlagSet("progress update", flag.ExitOnError)
		mangaID := fs.String("manga-id", "", "manga id")
		chapter := fs.Int("chapter", 0, "current chapter")
		status := fs.String("status", "reading", "status")
		_ = fs.Parse(args)
		if *mangaID == "" {
			log.Fatal("manga-id is required")
		}

		payload := map[string]any{
			"current_chapter": *chapter,
			"status":          *status,
		}
		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodPut, baseURL+"/users/library/"+url.PathEscape(*mangaID), token, payload, &resp); err != nil {
			log.Fatalf("update failed: %v", err)
		}
		printJSON(resp)
	case "history":
		fs := flag.NewFlagSet("progress history", flag.ExitOnError)
		limit := fs.Int("limit", 20, "page size")
		offset := fs.Int("offset", 0, "offset")
		_ = fs.Parse(args)

		u, err := url.Parse(baseURL + "/users/library")
		if err != nil {
			log.Fatalf("invalid base url: %v", err)
		}
		qv := u.Query()
		qv.Set("limit", fmt.Sprintf("%d", *limit))
		qv.Set("offset", fmt.Sprintf("%d", *offset))
		u.RawQuery = qv.Encode()

		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, u.String(), token, nil, &resp); err != nil {
			log.Fatalf("history failed: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub progress <update|history>")
	}
}

func handleSync(sub string, args []string) {
	switch sub {
	case "listen":
		fs := flag.NewFlagSet("sync listen", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:7070", "TCP sync server address")
		pretty := fs.Bool("pretty", true, "pretty print JSON events")
		_ = fs.Parse(args)
		for {
			if err := runSyncTCP(*addr, *pretty); err != nil {
				log.Printf("[sync] disconnected: %v", err)
			}
			time.Sleep(1 * time.Second)
		}
	default:
		log.Fatal("usage: mangahub sync listen")
	}
}

func handleNotify(baseURL, sub string, args []string) {
	switch sub {
	case "subscribe":
		fs := flag.NewFlagSet("notify subscribe", flag.ExitOnError)
		wsURL := fs.String("ws", "", "WebSocket URL (defaults to /ws on API host)")
		_ = fs.Parse(args)

		endpoint := *wsURL
		if endpoint == "" {
			var err error
			endpoint, err = websocketURL(baseURL, "/ws")
			if err != nil {
				log.Fatalf("ws url: %v", err)
			}
		}
		if err := runWebSocket(endpoint); err != nil {
			log.Fatalf("subscribe failed: %v", err)
		}
	default:
		log.Fatal("usage: mangahub notify subscribe")
	}
}

func handleChat(sub string, args []string) {
	switch sub {
	case "join":
		fs := flag.NewFlagSet("chat join", flag.ExitOnError)
		addr := fs.String("addr", "127.0.0.1:9090", "UDP chat server address")
		name := fs.String("name", "guest", "display name")
		_ = fs.Parse(args)
		if err := runChatUDP(*addr, *name); err != nil {
			log.Fatalf("chat join failed: %v", err)
		}
	default:
		log.Fatal("usage: mangahub chat join")
	}
}

func handleExport(ctx context.Context, client *http.Client, baseURL, sub string, args []string) {
	switch sub {
	case "json":
		fs := flag.NewFlagSet("export json", flag.ExitOnError)
		out := fs.String("out", "data/manga.json", "output JSON path")
		limit := fs.Int("limit", 200, "max titles to export")
		_ = fs.Parse(args)

		items, err := fetchManga(ctx, client, baseURL, *limit)
		if err != nil {
			log.Fatalf("export json failed: %v", err)
		}
		if err := writeJSON(*out, items); err != nil {
			log.Fatalf("write json failed: %v", err)
		}
		log.Printf("✅ exported %d titles to %s", len(items), *out)
	case "csv":
		fs := flag.NewFlagSet("export csv", flag.ExitOnError)
		out := fs.String("out", "data/manga.csv", "output CSV path")
		limit := fs.Int("limit", 200, "max titles to export")
		_ = fs.Parse(args)

		items, err := fetchManga(ctx, client, baseURL, *limit)
		if err != nil {
			log.Fatalf("export csv failed: %v", err)
		}
		if err := writeCSV(*out, items); err != nil {
			log.Fatalf("write csv failed: %v", err)
		}
		log.Printf("✅ exported %d titles to %s", len(items), *out)
	default:
		log.Fatal("usage: mangahub export <json|csv>")
	}
}

func runSyncTCP(addr string, pretty bool) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	log.Printf("[sync] connected to %s", addr)
	reader := bufio.NewScanner(conn)
	for reader.Scan() {
		line := reader.Bytes()
		if !pretty {
			fmt.Println(string(line))
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			fmt.Println(string(line))
			continue
		}
		b, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(b))
	}
	if err := reader.Err(); err != nil {
		return err
	}
	return os.ErrClosed
}

func runWebSocket(wsURL string) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("[notify] connected to %s", wsURL)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		fmt.Println(string(msg))
	}
}

func runChatUDP(addr, name string) error {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Printf("[chat] connected to %s as %s", addr, name)
	if _, err := fmt.Fprintf(conn, "JOIN %s\n", name); err != nil {
		return err
	}

	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			fmt.Println(string(buf[:n]))
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if _, err := fmt.Fprintf(conn, "%s: %s\n", name, text); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func fetchManga(ctx context.Context, client *http.Client, baseURL string, limit int) ([]models.MangaDB, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be > 0")
	}

	var out []models.MangaDB
	offset := 0
	for len(out) < limit {
		pageSize := 50
		if remaining := limit - len(out); remaining < pageSize {
			pageSize = remaining
		}
		u, err := url.Parse(baseURL + "/manga")
		if err != nil {
			return nil, err
		}
		qv := u.Query()
		qv.Set("limit", fmt.Sprintf("%d", pageSize))
		qv.Set("offset", fmt.Sprintf("%d", offset))
		u.RawQuery = qv.Encode()

		var resp mangaListResponse
		if err := doJSON(ctx, client, http.MethodGet, u.String(), "", nil, &resp); err != nil {
			return nil, err
		}
		if len(resp.Items) == 0 {
			break
		}
		out = append(out, resp.Items...)
		offset += len(resp.Items)
		if offset >= resp.Total {
			break
		}
	}

	return out, nil
}

func writeJSON(path string, items []models.MangaDB) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeCSV(path string, items []models.MangaDB) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write([]string{
		"id", "title", "author", "status", "total_chapters", "genres", "description", "cover_url",
	}); err != nil {
		return err
	}
	for _, item := range items {
		if err := writer.Write([]string{
			item.ID,
			item.Title,
			item.Author,
			item.Status,
			fmt.Sprintf("%d", item.TotalChapters),
			strings.Join(item.Genres, ","),
			item.Description,
			item.CoverURL,
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func doJSON(ctx context.Context, client *http.Client, method, endpoint, token string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s failed: %s", method, endpoint, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatalf("json: %v", err)
	}
	fmt.Println(string(b))
}

func defaultTokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./.mangahub-token.json"
	}
	return filepath.Join(home, ".mangahub", "token.json")
}

func saveToken(path, token string) error {
	if token == "" {
		return errors.New("empty token")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tokenData{Token: token}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var td tokenData
	if err := json.Unmarshal(data, &td); err != nil {
		return "", err
	}
	return strings.TrimSpace(td.Token), nil
}

func mustToken(path string) string {
	token, err := readToken(path)
	if err != nil {
		log.Fatalf("token not found, please login: %v", err)
	}
	if token == "" {
		log.Fatal("token empty, please login")
	}
	return token
}

func clearToken(path string) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

func websocketURL(baseURL, path string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	return (&url.URL{
		Scheme: scheme,
		Host:   u.Host,
		Path:   path,
	}).String(), nil
}

func printUsage() {
	fmt.Println("mangahub <command> [subcommand] [flags]")
	fmt.Println("commands:")
	fmt.Println("  auth login|register|logout")
	fmt.Println("  manga search|show")
	fmt.Println("  library add|remove|list")
	fmt.Println("  progress update|history")
	fmt.Println("  sync listen")
	fmt.Println("  notify subscribe")
	fmt.Println("  chat join")
	fmt.Println("  export json|csv")
}
