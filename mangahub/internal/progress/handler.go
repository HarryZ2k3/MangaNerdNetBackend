package progress

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mangahub/internal/auth"
	"mangahub/pkg/models"
)

type Handler struct {
	Repo *Repo
}

func NewHandler(repo *Repo) *Handler {
	return &Handler{Repo: repo}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/progress", h.list)
	rg.POST("/progress", h.add)
}

type addReq struct {
	MangaID string `json:"manga_id"`
	Chapter int    `json:"chapter"`
	Volume  *int   `json:"volume,omitempty"`
}

func (h *Handler) add(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req addReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	mangaID := strings.TrimSpace(req.MangaID)
	if mangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}
	if req.Chapter < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapter must be >= 0"})
		return
	}
	if req.Volume != nil && *req.Volume < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "volume must be >= 0"})
		return
	}

	entry := models.ProgressHistory{
		UserID:  claims.UserID,
		MangaID: mangaID,
		Chapter: req.Chapter,
		Volume:  req.Volume,
		At:      time.Now().UTC(),
	}

	if err := h.Repo.Add(c.Request.Context(), entry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}

	c.JSON(http.StatusOK, entry)
}

func (h *Handler) list(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	mangaID := strings.TrimSpace(c.Query("manga_id"))
	if mangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}

	limit := parseInt(c.Query("limit"), 50)
	offset := parseInt(c.Query("offset"), 0)

	items, total, err := h.Repo.List(c.Request.Context(), claims.UserID, mangaID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"items":  items,
	})
}

func parseInt(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
