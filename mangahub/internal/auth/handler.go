package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	Repo   *Repo
	Tokens TokenService
}

func NewHandler(repo *Repo, tokens TokenService) *Handler {
	return &Handler{Repo: repo, Tokens: tokens}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.register)
	rg.POST("/login", h.login)
	rg.POST("/change-password", AuthMiddleware(h.Tokens, h.Repo), h.changePassword)
	rg.POST("/logout", AuthMiddleware(h.Tokens, h.Repo), h.logout)
}

type registerReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if len(req.Username) < 3 || len(req.Username) > 30 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username must be 3-30 chars"})
		return
	}
	if !strings.Contains(req.Email, "@") || len(req.Email) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email"})
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 8-72 chars"})
		return
	}

	// uniqueness checks
	if u, _ := h.Repo.GetByEmail(c.Request.Context(), req.Email); u != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}
	if u, _ := h.Repo.GetByUsername(c.Request.Context(), req.Username); u != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
		return
	}

	u := User{
		ID:           uuid.NewString(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
	}

	if err := h.Repo.CreateUser(c.Request.Context(), u); err != nil {
		// SQLite unique constraint will also trigger here in races
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create user failed"})
		return
	}

	// auto-login
	created := &u
	token, exp, err := h.Tokens.Sign(created)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user": gin.H{
			"id":       created.ID,
			"username": created.Username,
			"email":    created.Email,
		},
		"token":      token,
		"expires_at": exp.UTC().Format(time.RFC3339),
	})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and password required"})
		return
	}

	u, err := h.Repo.GetByEmail(c.Request.Context(), email)
	if err != nil || u == nil {
		// don't reveal which part failed
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, exp, err := h.Tokens.Sign(u)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":       u.ID,
			"username": u.Username,
			"email":    u.Email,
		},
		"token":      token,
		"expires_at": exp.UTC().Format(time.RFC3339),
	})
}

type changePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *Handler) changePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old and new password required"})
		return
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be 8-72 chars"})
		return
	}

	claims := MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	u, err := h.Repo.GetByID(c.Request.Context(), claims.UserID)
	if err != nil || u == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
		return
	}

	if err := h.Repo.UpdatePasswordAndBumpTokenVersion(c.Request.Context(), u.ID, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update password failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "password updated"})
}

func (h *Handler) logout(c *gin.Context) {
	claims := MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	if err := h.Repo.BumpTokenVersion(c.Request.Context(), claims.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "logout failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "logged out"})
}
