package sub

import (
	"errors"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/subscription"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type projectProxySource struct {
	proxy        map[string]any
	nodeKey      string
	inboundKey   string
	inboundName  string
	hostOverride bool
}
type projectSubscription struct {
	binding *subscription.SubscriptionBinding
	inspect bool
	context bool
	sources []projectProxySource
}

func (s *SubClashService) ProjectSubscription(subID, host string, binding *subscription.SubscriptionBinding, context ...bool) (string, error) {
	project := &projectSubscription{binding: binding, inspect: binding == nil, context: len(context) > 0 && context[0]}
	body, _, err := s.getClashWithProject(subID, host, false, project)
	if err == nil && body == "" {
		if binding == nil {
			body = "[]"
			if project.context {
				req := NewSubService("")
				req.address = host
				raw, marshalErr := yaml.Marshal(map[string]any{"proxies": []any{}, "publication": subscription.PublicationSnapshot(subID, req.projectPublicBaseURL())})
				if marshalErr != nil {
					return "", marshalErr
				}
				body = string(raw)
			}
		} else {
			err = errors.New("订阅没有可用的授权节点")
		}
	}
	return body, err
}
func (p *projectSubscription) proxies(s *SubClashService, req *SubService, inbound *model.Inbound, client model.Client, host string) []map[string]any {
	items := s.getProxies(req, inbound, client, host)
	endpoints, _ := unmarshalStreamSettings(inbound.StreamSettings)["externalProxy"].([]any)
	hostOverride := false
	for _, value := range endpoints {
		if endpoint, ok := value.(map[string]any); ok && isHostEndpoint(endpoint) {
			hostOverride = true
			break
		}
	}
	key := catalog.LocalNodeKey
	if inbound.NodeID != nil {
		key = catalog.NodeKey(*inbound.NodeID)
	}
	for _, item := range items {
		p.sources = append(p.sources, projectProxySource{item, key, subscription.InboundKey(inbound.Id), inbound.Remark, hostOverride})
	}
	return items
}
func (p *projectSubscription) render(subID string, proxies []map[string]any, legacy bool, publicBaseURL string) (string, bool, error) {
	if legacy {
		return "", false, nil
	}
	nodeKeys := make(map[string]string, len(p.sources))
	inboundKeys := make(map[string]string, len(p.sources))
	inboundNames := make(map[string]string, len(p.sources))
	hostOverrides := make(map[string]bool, len(p.sources))
	for _, source := range p.sources {
		name, _ := source.proxy["name"].(string)
		nodeKeys[name] = source.nodeKey
		inboundKeys[name] = source.inboundKey
		inboundNames[name] = source.inboundName
		hostOverrides[name] = source.hostOverride
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
			name := fmt.Sprint(proxy["name"])
			items = append(items, map[string]any{
				"name": name, "type": proxy["type"], "config": proxy,
				"nodeKey": nodeKeys[name], "inboundKey": inboundKeys[name], "inboundName": inboundNames[name], "hostOverride": hostOverrides[name],
			})
		}
		var output any = items
		if p.context {
			output = map[string]any{"proxies": items, "publication": subscription.PublicationSnapshot(subID, publicBaseURL)}
		}
		body, err := yaml.Marshal(output)
		return string(body), true, err
	}
	config, bound, err := subscription.RenderManagedSubscription(subID, p.binding, actual, nodeKeys, inboundKeys, publicBaseURL)
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

// Use the official subscription URL builder so custom reverse-proxy prefixes are preserved.
func (s *SubService) projectPublicBaseURL() string {
	path, err := s.settingService.GetSubClashPath()
	if err != nil {
		return ""
	}
	_, _, address := s.BuildURLs("", "", path, "_project")
	return address
}
