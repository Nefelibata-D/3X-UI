package httpapi

import (
	"errors"
	"net"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/subscription"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/sub"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func (a *handlers) clientSubscription(c *gin.Context) {
	id, ok := a.clientID(c)
	if !ok {
		return
	}
	row, err := subscription.ClientSubscription(id)
	a.reply(c, "", row, err)
}

func (a *handlers) saveClientSubscription(c *gin.Context) {
	id, ok := a.clientID(c)
	if !ok {
		return
	}
	var row subscription.SubscriptionBinding
	var err error
	if err = c.ShouldBindJSON(&row); err == nil {
		if row.TemplateID != 0 {
			_, err = projectSubscription(c, id, &row)
		}
		if err == nil {
			err = subscription.SaveClientSubscription(id, &row)
		}
	}
	a.reply(c, "", row, err)
}

func projectSubscription(c *gin.Context, id int, binding *subscription.SubscriptionBinding) (any, error) {
	var client model.ClientRecord
	if err := database.GetDB().First(&client, id).Error; err != nil {
		return "", err
	}
	if client.SubID == "" {
		return "", errors.New("客户端没有订阅身份")
	}
	var settings service.SettingService
	remark, err := settings.GetRemarkTemplate()
	if err != nil {
		return "", err
	}
	host := c.Request.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	renderer := sub.NewSubClashService(false, "", false, sub.NewSubService(remark))
	if binding == nil {
		return renderer.ProjectProxies(client.SubID, host, c.Query("context") == "1")
	}
	return renderer.ProjectSubscription(client.SubID, host, binding, c.Query("context") == "1")
}

func (a *handlers) subscriptionProxies(c *gin.Context) {
	id, ok := a.clientID(c)
	if !ok {
		return
	}
	proxies, err := projectSubscription(c, id, nil)
	a.reply(c, "", proxies, err)
}

func (a *handlers) validateSubscription(c *gin.Context) {
	id, ok := a.clientID(c)
	if !ok {
		return
	}
	var binding subscription.SubscriptionBinding
	var err error
	var rendered any
	if err = c.ShouldBindJSON(&binding); err == nil {
		rendered, err = projectSubscription(c, id, &binding)
	}
	a.reply(c, "", rendered, err)
}

func (a *handlers) clientProfiles(c *gin.Context) {
	id, ok := a.clientID(c)
	if !ok {
		return
	}
	rows, err := subscription.ClientProfiles(id)
	a.reply(c, "", rows, err)
}

func (a *handlers) changeProfile(c *gin.Context) {
	id, ok := a.clientID(c)
	if !ok {
		return
	}
	var err error
	var change subscription.ProfileChange
	if err = c.ShouldBindJSON(&change); err == nil && !change.Delete && change.TemplateID != 0 {
		_, err = projectSubscription(c, id, &change.SubscriptionBinding)
	}
	if err != nil {
		a.reply(c, "", nil, err)
		return
	}
	row, err := subscription.ChangeProfile(id, change)
	a.reply(c, "", row, err)
}

func (a *handlers) clientID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		a.reply(c, "", nil, err)
	}
	return id, err == nil
}
