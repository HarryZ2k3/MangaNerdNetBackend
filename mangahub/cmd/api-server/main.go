package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"mangahub/internal/auth"
	"mangahub/internal/library"
	"mangahub/internal/manga"
	synchub "mangahub/internal/sync"
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
	hub := synchub.NewHub()
	router.GET("/ws", synchub.WSHandler(hub))
	tcpSrv := synchub.NewServer(":7070", hub)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": cfg.Path})
	})

	router.GET("/ready", func(c *gin.Context) {
		stats := hub.Stats()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":      "not_ready",
				"db_error":    err.Error(),
				"tcp_clients": stats.TCPClients,
				"ws_clients":  stats.WSClients,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":      "ready",
			"db":          "ok",
			"tcp_clients": stats.TCPClients,
			"ws_clients":  stats.WSClients,
		})
	})

	router.GET("/debug", func(c *gin.Context) {
		stats := hub.Stats()
		c.JSON(http.StatusOK, gin.H{
			"db":          cfg.Path,
			"tcp_clients": stats.TCPClients,
			"ws_clients":  stats.WSClients,
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

	httpSrv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := tcpSrv.Run(); err != nil {
			errCh <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("HTTP API server listening on :8080")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("shutdown signal received: %s", sig)
	case err := <-errCh:
		log.Printf("server error: %v", err)
	}

	log.Println("shutting down servers")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown error: %v", err)
	}
	if err := tcpSrv.Close(); err != nil {
		log.Printf("tcp shutdown error: %v", err)
	}

	wg.Wait()
	log.Println("servers stopped")
}
