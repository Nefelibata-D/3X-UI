package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"text/template"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/passkey"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/middleware"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/tgbot"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"

	"github.com/gin-gonic/gin"
)

func registerPasskeyManagement(api *gin.RouterGroup) {
	g := api.Group("/clash/passkeys")
	g.Use(func(c *gin.Context) {
		if c.GetBool("api_authed") || !session.IsLogin(c) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	g.GET("", passkeyList)
	g.POST("/domain", passkeyDomain)
	g.POST("/delete", passkeyDelete)
	g.POST("/register/begin", passkeyRegisterBegin)
	g.POST("/register/finish", passkeyRegisterFinish)
}

func (a *IndexController) registerPasskeyLogin(g *gin.RouterGroup) {
	p := g.Group("/passkey")
	p.Use(middleware.CSRFMiddleware(getRemoteIp))
	p.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if _, ok := defaultLoginLimiter.allow(getRemoteIp(c), "passkey"); !ok {
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		c.Next()
	})
	p.POST("/login/begin", a.passkeyLoginBegin)
	p.POST("/login/finish", a.passkeyLoginFinish)
}

func passkeyService() *passkey.Service { return passkey.New(database.GetDB()) }

func passkeyReply(c *gin.Context, obj any, err error) {
	if err == nil {
		jsonObj(c, obj, nil)
		return
	}
	key := "passkey.failed"
	for _, expected := range []error{passkey.ErrDomain, passkey.ErrOrigin, passkey.ErrCeremony, passkey.ErrCredential, passkey.ErrName} {
		if errors.Is(err, expected) {
			key = expected.Error()
			break
		}
	}
	if key == "passkey.failed" {
		logger.Warning("passkey operation failed:", err)
	}
	c.JSON(http.StatusOK, entity.Msg{Success: false, Msg: I18nWeb(c, key)})
}

func passkeyList(c *gin.Context) {
	view, err := passkeyService().List(session.GetLoginUser(c).Id)
	passkeyReply(c, view, err)
}

func passkeyDomain(c *gin.Context) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		passkeyReply(c, nil, passkey.ErrDomain)
		return
	}
	passkeyReply(c, nil, passkeyService().SetDomain(body.Domain))
}

func passkeyDelete(c *gin.Context) {
	var body struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		passkeyReply(c, nil, passkey.ErrCredential)
		return
	}
	passkeyReply(c, nil, passkeyService().Delete(session.GetLoginUser(c).Id, body.ID))
}

func passkeyRegisterBegin(c *gin.Context) {
	binding, err := session.EnsureCSRFToken(c)
	if err != nil {
		passkeyReply(c, nil, err)
		return
	}
	out, err := passkeyService().BeginRegistration(session.GetLoginUser(c).Id, c.GetHeader("Origin"), binding)
	passkeyReply(c, out, err)
}

type passkeyFinish struct {
	Name       string          `json:"name"`
	RequestID  string          `json:"requestId"`
	Credential json.RawMessage `json:"credential"`
}

func passkeyRegisterFinish(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var body passkeyFinish
	if err := c.ShouldBindJSON(&body); err != nil {
		passkeyReply(c, nil, passkey.ErrCredential)
		return
	}
	binding, err := session.EnsureCSRFToken(c)
	if err == nil {
		err = passkeyService().FinishRegistration(session.GetLoginUser(c).Id, c.GetHeader("Origin"), binding, body.RequestID, body.Name, body.Credential)
	}
	passkeyReply(c, nil, err)
}

func (a *IndexController) passkeyLoginBegin(c *gin.Context) {
	binding, err := session.EnsureCSRFToken(c)
	if err != nil {
		passkeyReply(c, nil, err)
		return
	}
	out, err := passkeyService().BeginLogin(c.GetHeader("Origin"), binding)
	if err != nil {
		defaultLoginLimiter.registerFailure(getRemoteIp(c), "passkey")
	}
	passkeyReply(c, out, err)
}

func (a *IndexController) passkeyLoginFinish(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var body passkeyFinish
	if err := c.ShouldBindJSON(&body); err != nil {
		defaultLoginLimiter.registerFailure(getRemoteIp(c), "passkey")
		passkeyReply(c, nil, passkey.ErrCredential)
		return
	}
	binding, err := session.EnsureCSRFToken(c)
	if err != nil {
		passkeyReply(c, nil, err)
		return
	}
	user, err := passkeyService().FinishLogin(c.GetHeader("Origin"), binding, body.RequestID, body.Credential)
	if err != nil {
		defaultLoginLimiter.registerFailure(getRemoteIp(c), "passkey")
		logger.Warningf("failed passkey login: IP=%q", getRemoteIp(c))
		passkeyReply(c, nil, err)
		return
	}
	if err = session.SetLoginUser(c, user); err != nil {
		passkeyReply(c, nil, err)
		return
	}
	defaultLoginLimiter.registerSuccess(getRemoteIp(c), "passkey")
	logger.Infof("logged in with passkey: username=%q, IP=%q", user.Username, getRemoteIp(c))
	a.tgbot.UserLoginNotify(tgbot.LoginAttempt{Username: template.HTMLEscapeString(user.Username), IP: getRemoteIp(c), Time: time.Now().Format("2006-01-02 15:04:05"), Status: tgbot.LoginSuccess})
	jsonMsg(c, I18nWeb(c, "pages.login.toasts.successLogin"), nil)
}
