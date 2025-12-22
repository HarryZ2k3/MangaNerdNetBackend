package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const CtxClaimsKey = "auth_claims"

func AuthMiddleware(tokens TokenService, repo *Repo) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			c.Abort()
			return
		}

		raw := strings.TrimSpace(h[len("Bearer "):])
		claims, err := tokens.Parse(raw)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		if repo != nil {
			currentVersion, err := repo.GetTokenVersion(c.Request.Context(), claims.UserID)
			if err != nil || currentVersion != claims.TokenVersion {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				c.Abort()
				return
			}
		}

		c.Set(CtxClaimsKey, claims)
		c.Next()
	}
}

func MustGetClaims(c *gin.Context) *Claims {
	v, ok := c.Get(CtxClaimsKey)
	if !ok {
		return nil
	}
	claims, _ := v.(*Claims)
	return claims
}
