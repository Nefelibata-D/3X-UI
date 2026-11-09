package controller

import (
	"errors"
	"net"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/subscription"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/sub"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func (a *ClashController) clientSubscription(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	row, err := subscription.ClientSubscription(id)
	jsonObj(c, row, err)
}

func (a *ClashController) saveClientSubscription(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	var row subscription.SubscriptionBinding
	if err = c.ShouldBindJSON(&row); err == nil {
		if row.TemplateID != 0 {
			_, err = projectSubscription(c, id, &row)
		}
		if err == nil {
			err = subscription.SaveClientSubscription(id, &row)
		}
	}
	jsonObj(c, row, err)
}

func projectSubscription(c *gin.Context, id int, binding *subscription.SubscriptionBinding) (string, error) {
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
	return renderer.ProjectSubscription(client.SubID, host, binding, c.Query("context") == "1")
}
func (a *ClashController) subscriptionProxies(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	text, err := projectSubscription(c, id, nil)
	var proxies any
	if err == nil {
		err = yaml.Unmarshal([]byte(text), &proxies)
	}
	jsonObj(c, proxies, err)
}
func (a *ClashController) validateSubscription(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	var binding subscription.SubscriptionBinding
	var rendered string
	if err = c.ShouldBindJSON(&binding); err == nil {
		rendered, err = projectSubscription(c, id, &binding)
	}
	jsonObj(c, rendered, err)
}
