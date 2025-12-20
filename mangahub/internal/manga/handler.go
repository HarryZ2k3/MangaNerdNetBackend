package manga

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	Repo *Repo
}

func NewHandler(repo *Repo) *Handler {
	return &Handler{Repo: repo}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)        // GET /manga
	rg.GET("/:id", h.getByID) // GET /manga/:id
}

func (h *Handler) list(c *gin.Context) {
	q := ListQuery{
		Q:      c.Query("q"),
		Status: c.Query("status"),
		Limit:  parseInt(c.Query("limit"), 20),
		Offset: parseInt(c.Query("offset"), 0),
	}

	// genres=Action,Drama OR genres=Action&genres=Drama
	genres := c.QueryArray("genres")
	if len(genres) == 0 {
		if s := c.Query("genres"); s != "" {
			genres = strings.Split(s, ",")
		}
	}
	q.Genres = genres

	total, err := h.Repo.Count(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count failed"})
		return
	}

	items, err := h.Repo.List(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":  total,
		"limit":  q.Limit,
		"offset": q.Offset,
		"items":  items,
	})
}

func (h *Handler) getByID(c *gin.Context) {
	id := c.Param("id")
	m, err := h.Repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get failed"})
		return
	}
	if m == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, m)
}

func parseInt(s string, def int) int {
	if strings.TrimSpace(s) == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
