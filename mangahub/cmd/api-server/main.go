package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"mangahub/internal/auth"
	"mangahub/internal/manga"
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

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": cfg.Path})
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

	// Protected routes placeholder (we’ll add /users next)
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

	log.Println("HTTP API server listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
