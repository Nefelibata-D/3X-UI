package httpapi

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/allocation"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
)

type handlers struct {
	reply func(*gin.Context, string, any, error)
}

func RegisterAdmin(g *gin.RouterGroup, reply func(*gin.Context, string, any, error)) {
	a := &handlers{reply: reply}
	c := g.Group("/clash")
	a.registerRuleSets(c)
	c.GET("/overview", a.overview)
	c.POST("/weights", a.saveWeights)
	c.GET("/clients/:id", a.clientRatio)
	c.POST("/clients/:id/ratio", a.saveClientRatio)
	c.GET("/clients/:id/subscription/profiles", requireClashFormat, a.clientProfiles)
	c.POST("/clients/:id/subscription/profiles", requireClashFormat, a.changeProfile)
	c.GET("/clients/:id/subscription", requireClashFormat, a.clientSubscription)
	c.POST("/clients/:id/subscription", requireClashFormat, a.saveClientSubscription)
	c.GET("/clients/:id/subscription/proxies", requireClashFormat, a.subscriptionProxies)
	c.POST("/clients/:id/subscription/validate", requireClashFormat, a.validateSubscription)
	c.POST("/policies/reset", a.resetPolicy)
	c.POST("/sync", a.sync)
	c.POST("/policy/apply", a.applyPolicy)
	c.GET("/policy/report", a.reportPolicy)
}

func (a *handlers) overview(c *gin.Context) {
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		a.reply(c, "load clash catalog", nil, err)
		return
	}
	out, err := policy.BuildOverview(cat)
	a.reply(c, "", out, err)
}

func (a *handlers) saveWeights(c *gin.Context) {
	var body struct {
		Items []allocation.NodeWeightInput `json:"items"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		a.reply(c, "invalid node weights", nil, err)
		return
	}
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		a.reply(c, "load clash catalog", nil, err)
		return
	}
	if err := allocation.SaveNodeWeights(cat, body.Items); err != nil {
		a.reply(c, "save node weights", nil, err)
		return
	}
	policy.TriggerSync()
	out, err := policy.BuildOverview(cat)
	a.reply(c, "", out, err)
}

func (a *handlers) clientRatio(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		a.reply(c, "invalid client id", nil, err)
		return
	}
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		a.reply(c, "load clash catalog", nil, err)
		return
	}
	view, err := policy.LoadClientView(cat, id)
	a.reply(c, "", view, err)
}

func (a *handlers) saveClientRatio(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		a.reply(c, "invalid client id", nil, err)
		return
	}
	var body struct {
		Mode      string           `json:"mode"`
		ResetMode string           `json:"resetMode"`
		Weights   map[string]int64 `json:"weights"`
		Limits    map[string]int64 `json:"limits"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		a.reply(c, "invalid client ratio", nil, err)
		return
	}
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		a.reply(c, "load clash catalog", nil, err)
		return
	}
	if err := allocation.SaveClientRatio(cat, id, body.Mode, body.Weights, body.Limits, body.ResetMode); err != nil {
		a.reply(c, "save client ratio", nil, err)
		return
	}
	policy.TriggerSync()
	view, err := policy.LoadClientView(cat, id)
	a.reply(c, "", view, err)
}

func (a *handlers) resetPolicy(c *gin.Context) {
	var body struct {
		NodeKey  string `json:"nodeKey"`
		ClientID int    `json:"clientId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		a.reply(c, "invalid reset request", nil, err)
		return
	}
	if err := policy.RequestReset(body.NodeKey, body.ClientID); err != nil {
		a.reply(c, "reset policy", nil, err)
		return
	}
	a.reply(c, "", gin.H{"resetPending": true}, nil)
}

func (a *handlers) sync(c *gin.Context) {
	rt := policy.GetRuntime()
	if rt == nil {
		a.reply(c, "clash runtime is not configured", nil, nil)
		return
	}
	rt.SyncOnce(c.Request.Context())
	a.reply(c, "", rt.SyncStatus(), nil)
}

func (a *handlers) applyPolicy(c *gin.Context) {
	var req policy.ApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		a.reply(c, "invalid policy", nil, err)
		return
	}
	rt := policy.GetRuntime()
	if rt == nil {
		a.reply(c, "clash runtime is not configured", nil, nil)
		return
	}
	report, err := rt.Apply(req)
	a.reply(c, "", report, err)
}

func (a *handlers) reportPolicy(c *gin.Context) {
	masterGuid := c.Query("masterGuid")
	id, err := strconv.Atoi(c.Query("clientId"))
	if err != nil || masterGuid == "" || id <= 0 {
		a.reply(c, "invalid report query", nil, err)
		return
	}
	rt := policy.GetRuntime()
	if rt == nil {
		a.reply(c, "clash runtime is not configured", nil, nil)
		return
	}
	report, err := rt.Report(masterGuid, id)
	a.reply(c, "", report, err)
}
