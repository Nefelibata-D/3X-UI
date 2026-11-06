package controller

import (
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/clash"

	"github.com/gin-gonic/gin"
)

type ClashController struct {
	BaseController
}

func NewClashController(g *gin.RouterGroup) *ClashController {
	a := &ClashController{}
	c := g.Group("/clash")
	registerRuleSets(c)
	c.GET("/overview", a.overview)
	c.POST("/weights", a.saveWeights)
	c.GET("/clients/:id", a.clientRatio)
	c.POST("/clients/:id/ratio", a.saveClientRatio)
	c.GET("/clients/:id/subscription", requireClashFormat, a.clientSubscription)
	c.POST("/clients/:id/subscription", requireClashFormat, a.saveClientSubscription)
	c.GET("/clients/:id/subscription/proxies", requireClashFormat, a.subscriptionProxies)
	c.POST("/clients/:id/subscription/validate", requireClashFormat, a.validateSubscription)
	c.POST("/policies/reset", a.resetPolicy)
	c.POST("/sync", a.sync)
	c.POST("/policy/apply", a.applyPolicy)
	c.GET("/policy/report", a.reportPolicy)
	return a
}

func (a *ClashController) overview(c *gin.Context) {
	cat, err := clash.LoadCatalogOrError()
	if err != nil {
		jsonMsg(c, "load clash catalog", err)
		return
	}
	out, err := clash.BuildOverview(cat)
	jsonObj(c, out, err)
}

func (a *ClashController) saveWeights(c *gin.Context) {
	var body struct {
		Items []clash.NodeWeightInput `json:"items"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, "invalid node weights", err)
		return
	}
	cat, err := clash.LoadCatalogOrError()
	if err != nil {
		jsonMsg(c, "load clash catalog", err)
		return
	}
	if err := clash.SaveNodeWeights(cat, body.Items); err != nil {
		jsonMsg(c, "save node weights", err)
		return
	}
	clash.TriggerSync()
	out, err := clash.BuildOverview(cat)
	jsonObj(c, out, err)
}

func (a *ClashController) clientRatio(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "invalid client id", err)
		return
	}
	cat, err := clash.LoadCatalogOrError()
	if err != nil {
		jsonMsg(c, "load clash catalog", err)
		return
	}
	view, err := clash.LoadClientView(cat, id)
	jsonObj(c, view, err)
}

func (a *ClashController) saveClientRatio(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "invalid client id", err)
		return
	}
	var body struct {
		Mode      string           `json:"mode"`
		ResetMode string           `json:"resetMode"`
		Weights   map[string]int64 `json:"weights"`
		Limits    map[string]int64 `json:"limits"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, "invalid client ratio", err)
		return
	}
	cat, err := clash.LoadCatalogOrError()
	if err != nil {
		jsonMsg(c, "load clash catalog", err)
		return
	}
	if err := clash.SaveClientRatio(cat, id, body.Mode, body.Weights, body.Limits, body.ResetMode); err != nil {
		jsonMsg(c, "save client ratio", err)
		return
	}
	clash.TriggerSync()
	view, err := clash.LoadClientView(cat, id)
	jsonObj(c, view, err)
}

func (a *ClashController) resetPolicy(c *gin.Context) {
	var body struct {
		NodeKey  string `json:"nodeKey"`
		ClientID int    `json:"clientId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, "invalid reset request", err)
		return
	}
	if err := clash.RequestReset(body.NodeKey, body.ClientID); err != nil {
		jsonMsg(c, "reset policy", err)
		return
	}
	jsonObj(c, gin.H{"resetPending": true}, nil)
}

func (a *ClashController) sync(c *gin.Context) {
	rt := clash.GetRuntime()
	if rt == nil {
		jsonMsg(c, "clash runtime is not configured", nil)
		return
	}
	rt.SyncOnce(c.Request.Context())
	jsonObj(c, rt.SyncStatus(), nil)
}

func (a *ClashController) applyPolicy(c *gin.Context) {
	var req clash.ApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonMsg(c, "invalid policy", err)
		return
	}
	rt := clash.GetRuntime()
	if rt == nil {
		jsonMsg(c, "clash runtime is not configured", nil)
		return
	}
	report, err := rt.Apply(req)
	jsonMsgObj(c, "", report, err)
}

func (a *ClashController) reportPolicy(c *gin.Context) {
	masterGuid := c.Query("masterGuid")
	id, err := strconv.Atoi(c.Query("clientId"))
	if err != nil || masterGuid == "" || id <= 0 {
		jsonMsg(c, "invalid report query", err)
		return
	}
	rt := clash.GetRuntime()
	if rt == nil {
		jsonMsg(c, "clash runtime is not configured", nil)
		return
	}
	report, err := rt.Report(masterGuid, id)
	jsonObj(c, report, err)
}
