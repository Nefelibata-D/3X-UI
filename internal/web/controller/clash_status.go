package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/clash"
)

// This route uses the official client UUID, not the administrator session.
func RegisterClashStatus(g *gin.RouterGroup) {
	g.GET("/v1/nodes/:node/status", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(clash.ClientStatus(c.Request.Context(), parts[1], c.Param("node")))
	})
}
