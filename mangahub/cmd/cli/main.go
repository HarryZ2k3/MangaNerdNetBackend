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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"mangahub/pkg/database"
	"mangahub/pkg/grpc/mangapb"
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
	configPath := configPathFromArgs(os.Args[1:])
	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	global := flag.NewFlagSet("mangahub", flag.ExitOnError)
	baseURL := global.String("api", cfg.APIBaseURL, "API base URL")
	configFlag := global.String("config", configPath, "config file path")
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
	case "init":
		handleInit(*configFlag, cfg, args[1:])
		return
	case "auth":
		handleAuth(ctx, client, *baseURL, *tokenPath, sub, args[2:])
	case "manga":
		handleManga(ctx, client, *baseURL, sub, args[2:])
	case "library":
		handleLibrary(ctx, client, *baseURL, *tokenPath, sub, args[2:])
	case "progress":
		handleProgress(ctx, client, *baseURL, *tokenPath, sub, args[2:])
	case "sync":
		handleSync(cfg, sub, args[2:])
	case "notify":
		handleNotify(ctx, client, cfg, *baseURL, *tokenPath, sub, args[2:])
	case "chat":
		handleChat(ctx, client, cfg, *baseURL, sub, args[2:])
	case "grpc":
		handleGrpc(cfg, sub, args[2:])
	case "server":
		handleServer(ctx, client, *baseURL, sub, args[2:])
	case "export":
		handleExport(ctx, client, *baseURL, sub, args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func handleInit(configPath string, cfg CLIConfig, args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "overwrite existing config")
	_ = fs.Parse(args)

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		log.Fatalf("create config dir: %v", err)
	}
	if _, err := os.Stat(configPath); err == nil && !*force {
		log.Fatalf("config already exists: %s (use -force to overwrite)", configPath)
	}

	if err := writeConfig(configPath, cfg); err != nil {
		log.Fatalf("write config: %v", err)
	}

	logDir := defaultLogDir()
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Fatalf("create logs dir: %v", err)
	}

	dbCfg := database.DefaultConfig()
	db, err := database.Open(dbCfg)
	if err != nil {
		log.Fatalf("create db: %v", err)
	}
	_ = db.Close()

	fmt.Printf("✅ initialized config at %s\n", configPath)
	fmt.Printf("✅ logs directory: %s\n", logDir)
	fmt.Printf("✅ database: %s\n", dbCfg.Path)
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
	case "status":
		token := mustToken(tokenPath)
		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, baseURL+"/users/me", token, nil, &resp); err != nil {
			log.Fatalf("status failed: %v", err)
		}
		printJSON(resp)
	case "change-password":
		fs := flag.NewFlagSet("auth change-password", flag.ExitOnError)
		oldPassword := fs.String("old", "", "current password")
		newPassword := fs.String("new", "", "new password")
		_ = fs.Parse(args)

		if *oldPassword == "" || *newPassword == "" {
			log.Fatal("old and new passwords are required")
		}

		token := mustToken(tokenPath)
		payload := map[string]string{"old_password": *oldPassword, "new_password": *newPassword}
		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodPost, baseURL+"/auth/change-password", token, payload, &resp); err != nil {
			log.Fatalf("change-password failed: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub auth <login|register|logout|status|change-password>")
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
	case "list":
		handleManga(ctx, client, baseURL, "search", args)
	case "info":
		handleManga(ctx, client, baseURL, "show", args)
	default:
		log.Fatal("usage: mangahub manga <search|show|list|info>")
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
	case "update":
		handleProgress(ctx, client, baseURL, tokenPath, "update", args)
	default:
		log.Fatal("usage: mangahub library <add|remove|list|update>")
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
		mangaID := fs.String("manga-id", "", "manga id")
		limit := fs.Int("limit", 20, "page size")
		offset := fs.Int("offset", 0, "offset")
		_ = fs.Parse(args)

		if *mangaID == "" {
			log.Fatal("manga-id is required")
		}

		u, err := url.Parse(baseURL + "/users/progress")
		if err != nil {
			log.Fatalf("invalid base url: %v", err)
		}
		qv := u.Query()
		qv.Set("manga_id", *mangaID)
		qv.Set("limit", fmt.Sprintf("%d", *limit))
		qv.Set("offset", fmt.Sprintf("%d", *offset))
		u.RawQuery = qv.Encode()

		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, u.String(), token, nil, &resp); err != nil {
			log.Fatalf("history failed: %v", err)
		}
		printJSON(resp)
	case "sync":
		fs := flag.NewFlagSet("progress sync", flag.ExitOnError)
		mangaID := fs.String("manga-id", "", "manga id")
		chapter := fs.Int("chapter", 0, "chapter")
		volume := fs.Int("volume", -1, "volume (optional)")
		_ = fs.Parse(args)

		if *mangaID == "" {
			log.Fatal("manga-id is required")
		}

		payload := map[string]any{
			"manga_id": *mangaID,
			"chapter":  *chapter,
		}
		if *volume >= 0 {
			payload["volume"] = *volume
		}

		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodPost, baseURL+"/users/progress", token, payload, &resp); err != nil {
			log.Fatalf("sync failed: %v", err)
		}
		printJSON(resp)
	case "sync-status":
		fs := flag.NewFlagSet("progress sync-status", flag.ExitOnError)
		mangaID := fs.String("manga-id", "", "manga id")
		_ = fs.Parse(args)
		if *mangaID == "" {
			log.Fatal("manga-id is required")
		}

		u, err := url.Parse(baseURL + "/users/progress")
		if err != nil {
			log.Fatalf("invalid base url: %v", err)
		}
		qv := u.Query()
		qv.Set("manga_id", *mangaID)
		qv.Set("limit", "1")
		qv.Set("offset", "0")
		u.RawQuery = qv.Encode()

		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, u.String(), token, nil, &resp); err != nil {
			log.Fatalf("sync-status failed: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub progress <update|history|sync|sync-status>")
	}
}

func handleSync(cfg CLIConfig, sub string, args []string) {
	switch sub {
	case "listen", "monitor":
		fs := flag.NewFlagSet("sync listen", flag.ExitOnError)
		addr := fs.String("addr", cfg.TCPAddr, "TCP sync server address")
		pretty := fs.Bool("pretty", true, "pretty print JSON events")
		_ = fs.Parse(args)
		for {
			if err := runSyncTCP(*addr, *pretty); err != nil {
				log.Printf("[sync] disconnected: %v", err)
			}
			time.Sleep(1 * time.Second)
		}
	case "connect":
		fs := flag.NewFlagSet("sync connect", flag.ExitOnError)
		addr := fs.String("addr", cfg.TCPAddr, "TCP sync server address")
		pretty := fs.Bool("pretty", true, "pretty print JSON events")
		_ = fs.Parse(args)
		if err := runSyncTCP(*addr, *pretty); err != nil {
			log.Fatalf("[sync] disconnected: %v", err)
		}
	case "status":
		fs := flag.NewFlagSet("sync status", flag.ExitOnError)
		addr := fs.String("addr", cfg.TCPAddr, "TCP sync server address")
		_ = fs.Parse(args)
		conn, err := net.DialTimeout("tcp", *addr, 2*time.Second)
		if err != nil {
			log.Fatalf("sync status failed: %v", err)
		}
		_ = conn.Close()
		fmt.Println("✅ sync server reachable")
	case "disconnect":
		fmt.Println("sync sessions run in the foreground; stop with Ctrl+C")
	default:
		log.Fatal("usage: mangahub sync <connect|disconnect|status|listen|monitor>")
	}
}

func handleNotify(ctx context.Context, client *http.Client, cfg CLIConfig, baseURL, tokenPath, sub string, args []string) {
	switch sub {
	case "subscribe":
		fs := flag.NewFlagSet("notify subscribe", flag.ExitOnError)
		userID := fs.String("user-id", "", "user id (defaults to current user)")
		udpAddr := fs.String("udp", cfg.UDPAddr, "UDP notify server address")
		_ = fs.Parse(args)

		resolvedUser := strings.TrimSpace(*userID)
		if resolvedUser == "" {
			token := mustToken(tokenPath)
			u, err := fetchUserID(ctx, client, baseURL, token)
			if err != nil {
				log.Fatalf("resolve user id: %v", err)
			}
			resolvedUser = u
		}

		if err := runNotifyUDP(*udpAddr, resolvedUser); err != nil {
			log.Fatalf("subscribe failed: %v", err)
		}
	case "unsubscribe":
		fs := flag.NewFlagSet("notify unsubscribe", flag.ExitOnError)
		userID := fs.String("user-id", "", "user id (defaults to current user)")
		udpAddr := fs.String("udp", cfg.UDPAddr, "UDP notify server address")
		_ = fs.Parse(args)

		resolvedUser := strings.TrimSpace(*userID)
		if resolvedUser == "" {
			token := mustToken(tokenPath)
			u, err := fetchUserID(ctx, client, baseURL, token)
			if err != nil {
				log.Fatalf("resolve user id: %v", err)
			}
			resolvedUser = u
		}

		if err := sendNotifyUnregister(*udpAddr, resolvedUser); err != nil {
			log.Fatalf("unsubscribe failed: %v", err)
		}
		fmt.Println("✅ unsubscribe request sent")
	case "preferences":
		fs := flag.NewFlagSet("notify preferences", flag.ExitOnError)
		mute := fs.Bool("mute", false, "mute notifications")
		unmute := fs.Bool("unmute", false, "unmute notifications")
		_ = fs.Parse(args)

		prefs, err := loadNotifyPreferences()
		if err != nil {
			log.Fatalf("load preferences: %v", err)
		}

		if *mute || *unmute {
			prefs.Muted = *mute && !*unmute
			if err := writeNotifyPreferences(prefs); err != nil {
				log.Fatalf("save preferences: %v", err)
			}
			fmt.Println("✅ preferences updated")
			return
		}

		printJSON(prefs)
	case "test":
		fs := flag.NewFlagSet("notify test", flag.ExitOnError)
		userID := fs.String("user-id", "", "user id (defaults to current user)")
		udpAddr := fs.String("udp", cfg.UDPAddr, "UDP notify server address")
		mangaID := fs.String("manga-id", "", "manga id")
		chapter := fs.Int("chapter", 1, "chapter number")
		_ = fs.Parse(args)

		resolvedUser := strings.TrimSpace(*userID)
		if resolvedUser == "" {
			token := mustToken(tokenPath)
			u, err := fetchUserID(ctx, client, baseURL, token)
			if err != nil {
				log.Fatalf("resolve user id: %v", err)
			}
			resolvedUser = u
		}
		if *mangaID == "" {
			log.Fatal("manga-id is required")
		}

		if err := sendNotifyTest(*udpAddr, resolvedUser, *mangaID, *chapter); err != nil {
			log.Fatalf("notify test failed: %v", err)
		}
		fmt.Println("✅ test notification sent")
	default:
		log.Fatal("usage: mangahub notify <subscribe|unsubscribe|preferences|test>")
	}
}

func handleChat(ctx context.Context, client *http.Client, cfg CLIConfig, baseURL string, sub string, args []string) {
	switch sub {
	case "join":
		fs := flag.NewFlagSet("chat join", flag.ExitOnError)
		room := fs.String("room", "lobby", "room name")
		name := fs.String("name", "guest", "display name")
		wsURL := fs.String("ws", "", "WebSocket URL (defaults to /ws/chat on API host)")
		_ = fs.Parse(args)
		endpoint := *wsURL
		if endpoint == "" {
			var err error
			endpoint, err = websocketURL(baseURL, "/ws/chat")
			if err != nil {
				log.Fatalf("ws url: %v", err)
			}
		}
		endpoint = addWSQuery(endpoint, map[string]string{
			"room": *room,
			"user": *name,
		})
		if err := runChatWebSocket(endpoint); err != nil {
			log.Fatalf("chat join failed: %v", err)
		}
	case "send":
		fs := flag.NewFlagSet("chat send", flag.ExitOnError)
		room := fs.String("room", "lobby", "room name")
		name := fs.String("name", "guest", "display name")
		text := fs.String("text", "", "message text")
		wsURL := fs.String("ws", "", "WebSocket URL (defaults to /ws/chat on API host)")
		_ = fs.Parse(args)
		if strings.TrimSpace(*text) == "" {
			log.Fatal("text is required")
		}
		endpoint := *wsURL
		if endpoint == "" {
			var err error
			endpoint, err = websocketURL(baseURL, "/ws/chat")
			if err != nil {
				log.Fatalf("ws url: %v", err)
			}
		}
		endpoint = addWSQuery(endpoint, map[string]string{
			"room": *room,
			"user": *name,
		})
		if err := sendChatWebSocket(endpoint, *text, *name); err != nil {
			log.Fatalf("chat send failed: %v", err)
		}
		fmt.Println("✅ message sent")
	case "history":
		fs := flag.NewFlagSet("chat history", flag.ExitOnError)
		room := fs.String("room", "lobby", "room name")
		_ = fs.Parse(args)
		u, err := url.Parse(baseURL + "/chat/history")
		if err != nil {
			log.Fatalf("invalid base url: %v", err)
		}
		qv := u.Query()
		qv.Set("room", *room)
		u.RawQuery = qv.Encode()
		var resp []map[string]any
		if err := doJSON(ctx, client, http.MethodGet, u.String(), "", nil, &resp); err != nil {
			log.Fatalf("chat history failed: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub chat <join|send|history>")
	}
}

func handleGrpc(cfg CLIConfig, sub string, args []string) {
	switch sub {
	case "manga":
		handleGrpcManga(cfg, args)
	case "progress":
		handleGrpcProgress(cfg, args)
	default:
		log.Fatal("usage: mangahub grpc <manga|progress>")
	}
}

func handleGrpcManga(cfg CLIConfig, args []string) {
	if len(args) == 0 {
		log.Fatal("usage: mangahub grpc manga <get|search>")
	}
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "get":
		fs := flag.NewFlagSet("grpc manga get", flag.ExitOnError)
		addr := fs.String("addr", cfg.GRPCAddr, "gRPC server address")
		id := fs.String("id", "", "manga id")
		_ = fs.Parse(rest)
		if *id == "" {
			log.Fatal("id is required")
		}

		conn, err := newGrpcConn(*addr)
		if err != nil {
			log.Fatalf("grpc connect: %v", err)
		}
		defer conn.Close()

		client := mangapb.NewMangaServiceClient(conn)
		resp, err := client.GetManga(context.Background(), &mangapb.GetMangaRequest{Id: *id})
		if err != nil {
			log.Fatalf("grpc get: %v", err)
		}
		printJSON(resp)
	case "search":
		fs := flag.NewFlagSet("grpc manga search", flag.ExitOnError)
		addr := fs.String("addr", cfg.GRPCAddr, "gRPC server address")
		query := fs.String("q", "", "search query")
		status := fs.String("status", "", "status filter")
		genres := fs.String("genres", "", "comma-separated genres")
		limit := fs.Int("limit", 20, "page size")
		offset := fs.Int("offset", 0, "offset")
		_ = fs.Parse(rest)

		var genreList []string
		if *genres != "" {
			genreList = strings.Split(*genres, ",")
		}

		conn, err := newGrpcConn(*addr)
		if err != nil {
			log.Fatalf("grpc connect: %v", err)
		}
		defer conn.Close()

		client := mangapb.NewMangaServiceClient(conn)
		resp, err := client.ListManga(context.Background(), &mangapb.ListMangaRequest{
			Q:      *query,
			Genres: genreList,
			Status: *status,
			Limit:  int32(*limit),
			Offset: int32(*offset),
		})
		if err != nil {
			log.Fatalf("grpc search: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub grpc manga <get|search>")
	}
}

func handleGrpcProgress(cfg CLIConfig, args []string) {
	if len(args) == 0 {
		log.Fatal("usage: mangahub grpc progress <update>")
	}
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "update":
		fs := flag.NewFlagSet("grpc progress update", flag.ExitOnError)
		addr := fs.String("addr", cfg.GRPCAddr, "gRPC server address")
		userID := fs.String("user-id", "", "user id")
		mangaID := fs.String("manga-id", "", "manga id")
		chapter := fs.Int("chapter", 0, "current chapter")
		status := fs.String("status", "reading", "status")
		_ = fs.Parse(rest)

		if *userID == "" || *mangaID == "" {
			log.Fatal("user-id and manga-id are required")
		}

		conn, err := newGrpcConn(*addr)
		if err != nil {
			log.Fatalf("grpc connect: %v", err)
		}
		defer conn.Close()

		client := mangapb.NewProgressServiceClient(conn)
		resp, err := client.UpsertProgress(context.Background(), &mangapb.UpsertProgressRequest{
			UserId:         *userID,
			MangaId:        *mangaID,
			CurrentChapter: int32(*chapter),
			Status:         *status,
		})
		if err != nil {
			log.Fatalf("grpc update: %v", err)
		}
		printJSON(resp)
	default:
		log.Fatal("usage: mangahub grpc progress <update>")
	}
}

func newGrpcConn(addr string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func handleServer(ctx context.Context, client *http.Client, baseURL, sub string, args []string) {
	switch sub {
	case "health", "ping":
		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, baseURL+"/health", "", nil, &resp); err != nil {
			log.Fatalf("health failed: %v", err)
		}
		printJSON(resp)
	case "status":
		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, baseURL+"/ready", "", nil, &resp); err != nil {
			log.Fatalf("status failed: %v", err)
		}
		printJSON(resp)
	case "logs":
		var resp map[string]any
		if err := doJSON(ctx, client, http.MethodGet, baseURL+"/debug", "", nil, &resp); err != nil {
			log.Fatalf("logs failed: %v", err)
		}
		printJSON(resp)
	case "start":
		fs := flag.NewFlagSet("server start", flag.ExitOnError)
		cmd := fs.String("cmd", "go run ./cmd/api-server", "command to start API server")
		_ = fs.Parse(args)
		if err := startServer(*cmd); err != nil {
			log.Fatalf("start server: %v", err)
		}
		fmt.Println("✅ server started")
	case "stop":
		if err := stopServer(); err != nil {
			log.Fatalf("stop server: %v", err)
		}
		fmt.Println("✅ server stopped")
	default:
		log.Fatal("usage: mangahub server <start|stop|status|health|logs|ping>")
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

type CLIConfig struct {
	APIBaseURL string `json:"api_base_url"`
	TCPAddr    string `json:"tcp_addr"`
	UDPAddr    string `json:"udp_addr"`
	GRPCAddr   string `json:"grpc_addr"`
	WSBaseURL  string `json:"ws_base_url"`
}

type NotifyPreferences struct {
	Muted bool `json:"muted"`
}

func defaultConfig() CLIConfig {
	return CLIConfig{
		APIBaseURL: defaultBaseURL,
		TCPAddr:    "127.0.0.1:7070",
		UDPAddr:    "127.0.0.1:6060",
		GRPCAddr:   "127.0.0.1:9090",
		WSBaseURL:  "",
	}
}

func loadConfig(path string) (CLIConfig, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = defaultBaseURL
	}
	if cfg.TCPAddr == "" {
		cfg.TCPAddr = "127.0.0.1:7070"
	}
	if cfg.UDPAddr == "" {
		cfg.UDPAddr = "127.0.0.1:6060"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = "127.0.0.1:9090"
	}
	return cfg, nil
}

func writeConfig(path string, cfg CLIConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./mangahub-config.json"
	}
	return filepath.Join(home, ".mangahub", "config.json")
}

func defaultLogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./mangahub-logs"
	}
	return filepath.Join(home, ".mangahub", "logs")
}

func notifyPreferencesPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./mangahub-notify.json"
	}
	return filepath.Join(home, ".mangahub", "notify.json")
}

func loadNotifyPreferences() (NotifyPreferences, error) {
	path := notifyPreferencesPath()
	var prefs NotifyPreferences
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return prefs, nil
		}
		return prefs, err
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return prefs, err
	}
	return prefs, nil
}

func writeNotifyPreferences(prefs NotifyPreferences) error {
	path := notifyPreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func configPathFromArgs(args []string) string {
	path := defaultConfigPath()
	for i := 0; i < len(args); i++ {
		if (args[i] == "-config" || args[i] == "--config") && i+1 < len(args) {
			path = args[i+1]
		}
	}
	return path
}

func fetchUserID(ctx context.Context, client *http.Client, baseURL, token string) (string, error) {
	var resp struct {
		ID string `json:"id"`
	}
	if err := doJSON(ctx, client, http.MethodGet, baseURL+"/users/me", token, nil, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.ID) == "" {
		return "", errors.New("missing user id in response")
	}
	return resp.ID, nil
}

func runNotifyUDP(addr, userID string) error {
	remote, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return err
	}
	defer conn.Close()

	msg := map[string]string{"type": "register", "user_id": userID}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := conn.Write(payload); err != nil {
		return err
	}
	log.Printf("[notify] registered user %s with %s", userID, addr)

	buffer := make([]byte, 2048)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return err
		}
		fmt.Println(string(buffer[:n]))
	}
}

func sendNotifyUnregister(addr, userID string) error {
	remote, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return err
	}
	defer conn.Close()
	msg := map[string]string{"type": "unregister", "user_id": userID}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

func sendNotifyTest(addr, userID, mangaID string, chapter int) error {
	remote, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return err
	}
	defer conn.Close()
	msg := map[string]any{
		"type":     "new_chapter",
		"user_id":  userID,
		"manga_id": mangaID,
		"chapter":  chapter,
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

func addWSQuery(endpoint string, values map[string]string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	q := u.Query()
	for key, value := range values {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func runChatWebSocket(wsURL string) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("[chat] connected to %s", wsURL)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			fmt.Println(string(msg))
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		payload := map[string]string{"text": text}
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	<-done
	return nil
}

func sendChatWebSocket(wsURL, text, user string) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	payload := map[string]string{"text": text, "user": user}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, b)
}

func serverPIDPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./mangahub-server.pid"
	}
	return filepath.Join(home, ".mangahub", "server.pid")
}

func startServer(command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return errors.New("command is empty")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(serverPIDPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(serverPIDPath(), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o600)
}

func stopServer() error {
	data, err := os.ReadFile(serverPIDPath())
	if err != nil {
		return err
	}
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		return errors.New("missing pid")
	}
	parsedPID := parsePID(pid)
	if parsedPID <= 0 {
		return errors.New("invalid pid")
	}
	proc, err := os.FindProcess(parsedPID)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	return os.Remove(serverPIDPath())
}

func parsePID(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func printUsage() {
	fmt.Println("mangahub <command> [subcommand] [flags]")
	fmt.Println("commands:")
	fmt.Println("  init")
	fmt.Println("  auth login|register|logout|status|change-password")
	fmt.Println("  manga search|show|list|info")
	fmt.Println("  library add|remove|list|update")
	fmt.Println("  progress update|history|sync|sync-status")
	fmt.Println("  sync connect|disconnect|status|listen|monitor")
	fmt.Println("  notify subscribe|unsubscribe|preferences|test")
	fmt.Println("  chat join|send|history")
	fmt.Println("  grpc manga get|search; grpc progress update")
	fmt.Println("  server start|stop|status|health|logs|ping")
	fmt.Println("  export json|csv")
}
