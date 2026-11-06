package controller

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/clash"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"gorm.io/gorm"
	"net/http"
	"strconv"
)

func requireClashFormat(c *gin.Context) {
	var settings service.SettingService
	enabled, err := settings.GetSubClashEnable()
	if err != nil {
		c.AbortWithStatus(500)
		return
	}
	if !enabled {
		c.AbortWithStatus(404)
		return
	}
	c.Next()
}

func registerRuleSets(admin *gin.RouterGroup) {
	templates := admin.Group("/templates", requireClashFormat)
	templates.GET("", func(c *gin.Context) { rows, err := clash.Templates(); jsonObj(c, rows, err) })
	templates.DELETE("/:id", func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err == nil {
			err = clash.DeleteTemplate(uint(id))
		}
		jsonObj(c, nil, err)
	})
	templates.POST("", func(c *gin.Context) {
		var row clash.YAMLTemplate
		err := c.ShouldBindJSON(&row)
		if err == nil {
			err = clash.SaveTemplate(&row)
		}
		jsonObj(c, row, err)
	})
	g := admin.Group("/rulesets", requireClashFormat)
	g.GET("", func(c *gin.Context) { rows, err := clash.RuleSets(); jsonObj(c, rows, err) })
	g.POST("", func(c *gin.Context) {
		var row clash.RuleSet
		err := c.ShouldBindJSON(&row)
		if err == nil {
			err = clash.SaveRuleSet(&row)
		}
		jsonObj(c, row, err)
	})
	g.POST("/import", func(c *gin.Context) {
		var in struct{ Content string }
		if err := c.ShouldBindJSON(&in); err != nil {
			jsonObj(c, nil, err)
			return
		}
		rules, err := clash.ImportRules(in.Content)
		jsonObj(c, rules, err)
	})
}

func RegisterPublishedRules(g *gin.RouterGroup) {
	g.GET("/rulesets/:id/:filename", requireClashFormat, func(c *gin.Context) {
		data, err := clash.PublishedRuleFile(c.Param("id"), c.Param("filename"))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.Status(404)
			return
		}
		if err != nil {
			c.Status(500)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
	})
	g.GET("/rulesets/:id", requireClashFormat, func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 32)
		if err != nil || id == 0 {
			c.Status(404)
			return
		}
		data, err := clash.PublishedRules(uint(id))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.Status(404)
			return
		}
		if err != nil {
			c.Status(500)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
	})
}
