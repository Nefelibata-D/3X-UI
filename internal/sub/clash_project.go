package sub

import (
	"errors"
	"fmt"
	"github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/clash"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type projectProxySource struct {
	proxy   map[string]any
	nodeKey string
}
type projectSubscription struct {
	binding *clash.SubscriptionBinding
	inspect bool
	sources []projectProxySource
}

func (s *SubClashService) ProjectSubscription(subID, host string, binding *clash.SubscriptionBinding) (string, error) {
	project := &projectSubscription{binding: binding, inspect: binding == nil}
	body, _, err := s.getClashWithProject(subID, host, false, project)
	if err == nil && body == "" {
		if binding == nil {
			body = "[]"
		} else {
			err = errors.New("订阅没有可用的授权节点")
		}
	}
	return body, err
}
func (p *projectSubscription) proxies(s *SubClashService, req *SubService, inbound *model.Inbound, client model.Client, host string) []map[string]any {
	items := s.getProxies(req, inbound, client, host)
	key := clash.LocalNodeKey
	if inbound.NodeID != nil {
		key = clash.NodeKey(*inbound.NodeID)
	}
	for _, item := range items {
		p.sources = append(p.sources, projectProxySource{item, key})
	}
	return items
}
func (p *projectSubscription) render(subID string, proxies []map[string]any, legacy bool) (string, bool, error) {
	if legacy {
		return "", false, nil
	}
	nodeKeys := make(map[string]string, len(p.sources))
	for _, source := range p.sources {
		name, _ := source.proxy["name"].(string)
		nodeKeys[name] = source.nodeKey
	}
	actual := make([]map[string]any, 0, len(proxies))
	for _, proxy := range proxies {
		if !isDummyProxy(proxy) {
			actual = append(actual, proxy)
		}
	}
	if p.inspect {
		items := make([]map[string]any, 0, len(actual))
		for _, proxy := range actual {
			items = append(items, map[string]any{"name": proxy["name"], "type": proxy["type"], "nodeKey": nodeKeys[fmt.Sprint(proxy["name"])], "config": proxy})
		}
		body, err := yaml.Marshal(items)
		return string(body), true, err
	}
	var config map[string]any
	var bound bool
	var err error
	if p.binding != nil {
		config, bound, err = clash.RenderSubscriptionBinding(*p.binding, actual, nodeKeys)
	} else {
		config, bound, err = clash.RenderSubscription(subID, actual, nodeKeys)
	}
	if err != nil {
		return "", true, err
	}
	if !bound && p.binding == nil {
		return "", false, nil
	}
	if err := validateClashRouteGraph(config); err != nil {
		return "", true, err
	}
	body, err := marshalClashYAML(config)
	return string(body), true, err
}
