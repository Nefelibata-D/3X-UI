package subscription

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
)

// Public routes share one implementation on the panel and subscription listeners.
func RegisterPublicSubscriptionRoutes(g *gin.RouterGroup) {
	health := func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Status(http.StatusNoContent) }
	g.GET("/health", health)
	g.HEAD("/health", health)
	g.GET("/v1/nodes/:node/status", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(policy.ClientStatus(c.Request.Context(), parts[1], c.Param("node")))
	})
}
