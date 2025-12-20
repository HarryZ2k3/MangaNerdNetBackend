package library

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mangahub/internal/auth"
	"mangahub/internal/sync"
	"mangahub/pkg/models"
)

type Handler struct {
	Repo *Repo
	Hub  *sync.Hub
}

func NewHandler(repo *Repo, hub *sync.Hub) *Handler {
	return &Handler{Repo: repo, Hub: hub}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/library", h.list)
	rg.POST("/library", h.addOrUpdate)
	rg.PUT("/library/:manga_id", h.addOrUpdate)
	rg.DELETE("/library/:manga_id", h.remove)
	rg.GET("/library/:manga_id", h.getOne)
}

type upsertReq struct {
	MangaID        string `json:"manga_id"` // required for POST
	CurrentChapter int    `json:"current_chapter"`
	Status         string `json:"status"`
}

func (h *Handler) addOrUpdate(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req upsertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	mangaID := strings.TrimSpace(req.MangaID)
	if mangaID == "" {
		mangaID = strings.TrimSpace(c.Param("manga_id"))
	}
	if mangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}

	status := normalizeStatus(req.Status)
	if status == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "status must be one of: reading, completed, wish_list, blacklist",
		})
		return
	}

	if req.CurrentChapter < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current_chapter must be >= 0"})
		return
	}

	if status == "blacklist" && req.CurrentChapter != 0 {
		req.CurrentChapter = 0
	}

	item := authToItem(claims.UserID, mangaID, req.CurrentChapter, status)
	if err := h.Repo.Upsert(c.Request.Context(), item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}

	// Return canonical stored row including updated_at
	saved, err := h.Repo.Get(c.Request.Context(), claims.UserID, mangaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch saved failed"})
		return
	}
	if saved == nil {
		// should not happen, but safe
		saved = &models.LibraryItem{
			UserID:         claims.UserID,
			MangaID:        mangaID,
			CurrentChapter: req.CurrentChapter,
			Status:         status,
			UpdatedAt:      time.Now().UTC(),
		}
	}

	if h.Hub != nil {
		ev := sync.LibraryEvent{
			Type:           "library.update",
			UserID:         claims.UserID,
			MangaID:        mangaID,
			CurrentChapter: saved.CurrentChapter,
			Status:         saved.Status,
			At:             time.Now().UTC(),
		}
		go h.Hub.BroadcastJSON(ev)
	}

	c.JSON(http.StatusOK, saved)
}

func (h *Handler) list(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	if status != "" {
		status = normalizeStatus(status)
		if status == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status filter"})
			return
		}
	}

	limit := parseInt(c.Query("limit"), 20)
	offset := parseInt(c.Query("offset"), 0)

	items, total, err := h.Repo.List(c.Request.Context(), claims.UserID, status, limit, offset)
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

func (h *Handler) remove(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	mangaID := strings.TrimSpace(c.Param("manga_id"))
	if mangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}

	ok, err := h.Repo.Delete(c.Request.Context(), claims.UserID, mangaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	if h.Hub != nil {
		ev := sync.LibraryEvent{
			Type:    "library.delete",
			UserID:  claims.UserID,
			MangaID: mangaID,
			At:      time.Now().UTC(),
		}
		go h.Hub.BroadcastJSON(ev)
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *Handler) getOne(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	mangaID := strings.TrimSpace(c.Param("manga_id"))
	if mangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}

	it, err := h.Repo.Get(c.Request.Context(), claims.UserID, mangaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get failed"})
		return
	}
	if it == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, it)
}

func normalizeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "reading":
		return "reading"
	case "completed":
		return "completed"
	case "wish list", "wish_list", "wishlist":
		return "wish_list"
	case "blacklist", "black_list", "black list":
		return "blacklist"
	default:
		return ""
	}
}

func authToItem(userID, mangaID string, chapter int, status string) (it models.LibraryItem) {
	it.UserID = userID
	it.MangaID = mangaID
	it.CurrentChapter = chapter
	it.Status = status
	return
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
