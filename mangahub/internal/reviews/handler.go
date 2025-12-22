package reviews

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"mangahub/internal/auth"
)

type Handler struct {
	Repo *Repo
}

func NewHandler(repo *Repo) *Handler {
	return &Handler{Repo: repo}
}

func (h *Handler) RegisterPublicRoutes(rg *gin.RouterGroup) {
	rg.GET("/manga/:id/reviews", h.listByManga)
}

func (h *Handler) RegisterProtectedRoutes(rg *gin.RouterGroup) {
	rg.POST("/reviews", h.create)
	rg.DELETE("/reviews/:id", h.delete)
}

type createReq struct {
	MangaID string `json:"manga_id"`
	Rating  int    `json:"rating"`
	Text    string `json:"text"`
}

func (h *Handler) create(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req createReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	mangaID := strings.TrimSpace(req.MangaID)
	if mangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga_id required"})
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be between 1 and 5"})
		return
	}

	review, err := h.Repo.Create(c.Request.Context(), claims.UserID, mangaID, req.Rating, strings.TrimSpace(req.Text))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed"})
		return
	}

	c.JSON(http.StatusCreated, review)
}

func (h *Handler) listByManga(c *gin.Context) {
	mangaID := strings.TrimSpace(c.Param("id"))
	if mangaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "manga id required"})
		return
	}

	limit := parseInt(c.Query("limit"), 20)
	offset := parseInt(c.Query("offset"), 0)

	reviews, err := h.Repo.ListByManga(c.Request.Context(), mangaID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"limit":  limit,
		"offset": offset,
		"items":  reviews,
	})
}

func (h *Handler) delete(c *gin.Context) {
	claims := auth.MustGetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idRaw := strings.TrimSpace(c.Param("id"))
	if idRaw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id required"})
		return
	}

	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	ok, err := h.Repo.Delete(c.Request.Context(), id, claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
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
