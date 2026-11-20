package sub

import (
	"errors"

	"github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/subscription"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type projectSubscription struct {
	binding  *subscription.SubscriptionBinding
	download bool
	preview  any
	inspect  bool
	context  bool
	sources  []subscription.ProxySource
}

func (s *SubClashService) ProjectSubscription(subID, host string, binding *subscription.SubscriptionBinding, context ...bool) (string, error) {
	if binding == nil {
		data, err := s.ProjectProxies(subID, host, len(context) > 0 && context[0])
		if err != nil {
			return "", err
		}
		raw, err := yaml.Marshal(data)
		if err != nil {
			return "", err
		}
		if list, ok := data.([]map[string]any); ok && len(list) == 0 {
			return "[]", nil
		}
		return string(raw), nil
	}
	body, _, err := s.getClashWithProject(subID, host, false, &projectSubscription{binding: binding})
	if err == nil && body == "" {
		err = errors.New("订阅没有可用的授权节点")
	}
	return body, err
}
func (s *SubClashService) ProjectProxies(subID, host string, withContext bool) (any, error) {
	p := &projectSubscription{inspect: true, context: withContext}
	_, _, err := s.getClashWithProject(subID, host, false, p)
	if err != nil {
		return nil, err
	}
	if p.preview == nil {
		req := NewSubService("")
		req.address = host
		p.preview = subscription.ProxyPreview(subID, req.projectPublicBaseURL(), withContext, nil, nil)
	}
	return p.preview, nil
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
		p.sources = append(p.sources, subscription.ProxySource{Proxy: item, NodeKey: key, InboundKey: subscription.InboundKey(inbound.Id), InboundName: inbound.Remark, HostOverride: hostOverride})
	}
	return items
}
func (p *projectSubscription) render(subID string, proxies []map[string]any, legacy bool, publicBaseURL string) (string, bool, error) {
	if legacy {
		return "", false, nil
	}
	actual := make([]map[string]any, 0, len(proxies))
	for _, proxy := range proxies {
		if !isDummyProxy(proxy) {
			actual = append(actual, proxy)
		}
	}
	if p.inspect {
		p.preview = subscription.ProxyPreview(subID, publicBaseURL, p.context, actual, p.sources)
		return "", true, nil
	}
	nodeKeys := make(map[string]string, len(p.sources))
	inboundKeys := make(map[string]string, len(p.sources))
	for _, source := range p.sources {
		name, _ := source.Proxy["name"].(string)
		nodeKeys[name] = source.NodeKey
		inboundKeys[name] = source.InboundKey
	}
	config, bound, err := subscription.RenderManagedSubscription(subID, p.binding, actual, nodeKeys, inboundKeys, publicBaseURL)
	if err != nil {
		return "", true, err
	}
	if !bound && (p.binding == nil || p.download) {
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
