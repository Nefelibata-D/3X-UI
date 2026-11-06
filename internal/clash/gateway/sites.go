package gateway

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// SiteGroup mirrors the client's explicit site grouping contract: exact
// domains win, then the longest matching suffix. The name is the snapshot key.
type SiteGroup struct {
	Name     string   `json:"name"`
	Domains  []string `json:"domains,omitempty"`
	Suffixes []string `json:"domainSuffixes,omitempty"`
}

type siteSuffixRule struct {
	domain string
	group  string
}

type siteGroupIndex struct {
	exact    map[string]string
	suffixes []siteSuffixRule
}

func newSiteGroupIndex(groups []SiteGroup) (*siteGroupIndex, error) {
	index := &siteGroupIndex{exact: map[string]string{}}
	names := make(map[string]struct{}, len(groups))
	suffixOwners := make(map[string]string)

	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			return nil, errors.New("gateway: site group name must not be empty")
		}
		if _, exists := names[name]; exists {
			return nil, fmt.Errorf("gateway: duplicate site group %q", name)
		}
		names[name] = struct{}{}
		if len(group.Domains) == 0 && len(group.Suffixes) == 0 {
			return nil, fmt.Errorf("gateway: site group %q has no domains or suffixes", name)
		}

		for _, raw := range group.Domains {
			domain, err := normalizeDomain(raw)
			if err != nil {
				return nil, fmt.Errorf("gateway: site group %q: %w", name, err)
			}
			if owner, exists := index.exact[domain]; exists && owner != name {
				return nil, fmt.Errorf("gateway: domain %q belongs to site groups %q and %q", domain, owner, name)
			}
			index.exact[domain] = name
		}
		for _, raw := range group.Suffixes {
			domain, err := normalizeDomain(raw)
			if err != nil {
				return nil, fmt.Errorf("gateway: site group %q: %w", name, err)
			}
			if owner, exists := suffixOwners[domain]; exists && owner != name {
				return nil, fmt.Errorf("gateway: suffix %q belongs to site groups %q and %q", domain, owner, name)
			}
			suffixOwners[domain] = name
		}
	}

	for domain, group := range suffixOwners {
		index.suffixes = append(index.suffixes, siteSuffixRule{domain: domain, group: group})
	}
	sort.Slice(index.suffixes, func(i, j int) bool {
		if len(index.suffixes[i].domain) != len(index.suffixes[j].domain) {
			return len(index.suffixes[i].domain) > len(index.suffixes[j].domain)
		}
		return index.suffixes[i].domain < index.suffixes[j].domain
	})
	return index, nil
}

func (index *siteGroupIndex) match(host string) (string, bool) {
	if index == nil {
		return "", false
	}
	if group, ok := index.exact[host]; ok {
		return group, true
	}
	for _, rule := range index.suffixes {
		if host == rule.domain || strings.HasSuffix(host, "."+rule.domain) {
			return rule.group, true
		}
	}
	return "", false
}

func siteKey(host string, index *siteGroupIndex) string {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return "ip:" + addr.Unmap().String()
	}
	domain, err := normalizeDomain(host)
	if err != nil {
		return "host:" + strings.ToLower(host)
	}
	if group, ok := index.match(domain); ok {
		return "group:" + group
	}
	etld, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		etld = domain
	}
	return "domain:" + etld
}

func normalizeDomain(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if domain == "" {
		return "", errors.New("domain must not be empty")
	}
	if strings.ContainsAny(domain, " \t\r\n/\\@") {
		return "", fmt.Errorf("invalid domain %q", raw)
	}
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		return "", fmt.Errorf("invalid domain %q: %w", raw, err)
	}
	ascii = strings.TrimSuffix(ascii, ".")
	if ascii == "" {
		return "", fmt.Errorf("invalid domain %q", raw)
	}
	return ascii, nil
}
