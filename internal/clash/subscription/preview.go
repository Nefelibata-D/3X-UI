package subscription

import "fmt"

type ProxySource struct {
	Proxy        map[string]any
	NodeKey      string
	InboundKey   string
	InboundName  string
	HostOverride bool
}

func ProxyPreview(subID, publicBaseURL string, withContext bool, actual []map[string]any, sources []ProxySource) any {
	byName := make(map[string]ProxySource, len(sources))
	for _, source := range sources {
		name, _ := source.Proxy["name"].(string)
		byName[name] = source
	}
	items := make([]map[string]any, 0, len(actual))
	for _, proxy := range actual {
		name := fmt.Sprint(proxy["name"])
		source := byName[name]
		items = append(items, map[string]any{
			"name": name, "type": proxy["type"], "config": proxy,
			"nodeKey": source.NodeKey, "inboundKey": source.InboundKey, "inboundName": source.InboundName, "hostOverride": source.HostOverride,
		})
	}
	if withContext {
		return map[string]any{"proxies": items, "publication": PublicationSnapshot(subID, publicBaseURL)}
	}
	return items
}
