package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/subscription"
)

// Project public endpoints own their handlers outside the upstream controller.
func RegisterClashStatus(g *gin.RouterGroup) {
	subscription.RegisterPublicSubscriptionRoutes(g)
}
