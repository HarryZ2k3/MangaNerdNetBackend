package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"mangahub/internal/auth"
	"mangahub/internal/library"
	"mangahub/internal/manga"
	"mangahub/internal/notify"
	"mangahub/internal/sync"
	"mangahub/pkg/database"
	"mangahub/pkg/utils"
)

func main() {
	cfg := database.DefaultConfig()
	db := database.MustOpen(cfg)
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("db migrate failed: %v", err)
	}

	router := gin.Default()

	// Optional: avoid “trusted all proxies” warning
	_ = router.SetTrustedProxies([]string{"127.0.0.1"})

	// Start TCP sync first (so you notice binding errors early)
	hub := sync.NewHub()
	router.GET("/ws", sync.WSHandler(hub))
	tcpSrv := sync.NewServer(":7070", hub)
	notifyRegistry := notify.NewRegistry()
	notifyServer := notify.NewServer(":9090", notifyRegistry, log.Default())

	errCh := make(chan error, 3)
	go func() { errCh <- tcpSrv.Run() }()
	go func() { errCh <- notifyServer.Run() }()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": cfg.Path})
	})

	router.GET("/debug", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"db":          cfg.Path,
			"tcp_clients": hub.Count(),
		})
	})

	// Manga (public)
	mangaRepo := manga.NewRepo(db)
	mangaHandler := manga.NewHandler(mangaRepo)
	mangaHandler.RegisterRoutes(router.Group("/manga"))

	// Auth
	authCfg := utils.LoadAuthConfig()
	tokenSvc := auth.TokenService{
		Secret:   []byte(authCfg.JWTSecret),
		Issuer:   authCfg.JWTIssuer,
		Duration: authCfg.JWTDuration,
	}
	authRepo := auth.NewRepo(db)
	authHandler := auth.NewHandler(authRepo, tokenSvc)
	authHandler.RegisterRoutes(router.Group("/auth"))

	// Protected routes
	protected := router.Group("/users")
	protected.Use(auth.AuthMiddleware(tokenSvc))

	protected.GET("/me", func(c *gin.Context) {
		claims := auth.MustGetClaims(c)
		c.JSON(http.StatusOK, gin.H{
			"id":       claims.UserID,
			"username": claims.Username,
			"email":    claims.Email,
		})
	})

	// Library (protected)
	libRepo := library.NewRepo(db)
	libHandler := library.NewHandler(libRepo, hub)
	libHandler.RegisterRoutes(protected)

	router.POST("/notify/release", func(c *gin.Context) {
		var payload struct {
			MangaID string `json:"manga_id"`
			Chapter int    `json:"chapter"`
		}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if payload.MangaID == "" || payload.Chapter <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id and chapter are required"})
			return
		}
		notifyServer.BroadcastNewChapter(payload.MangaID, payload.Chapter)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	log.Println("HTTP API server listening on :8080")
	go func() { errCh <- router.Run(":8080") }()

	// Single place to exit if any server fails
	log.Fatalf("server stopped: %v", <-errCh)
}
