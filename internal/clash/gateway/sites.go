package gateway

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/idna"
)

// SiteGroup preserves the existing policy wire format for older callers.
type SiteGroup struct {
	Name     string   `json:"name"`
	Domains  []string `json:"domains,omitempty"`
	Suffixes []string `json:"domainSuffixes,omitempty"`
}

func validateSiteGroups(groups []SiteGroup) error {
	exact := make(map[string]string)
	names := make(map[string]struct{}, len(groups))
	suffixOwners := make(map[string]string)

	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			return errors.New("gateway: site group name must not be empty")
		}
		if _, exists := names[name]; exists {
			return fmt.Errorf("gateway: duplicate site group %q", name)
		}
		names[name] = struct{}{}
		if len(group.Domains) == 0 && len(group.Suffixes) == 0 {
			return fmt.Errorf("gateway: site group %q has no domains or suffixes", name)
		}

		for _, raw := range group.Domains {
			domain, err := normalizeDomain(raw)
			if err != nil {
				return fmt.Errorf("gateway: site group %q: %w", name, err)
			}
			if owner, exists := exact[domain]; exists && owner != name {
				return fmt.Errorf("gateway: domain %q belongs to site groups %q and %q", domain, owner, name)
			}
			exact[domain] = name
		}
		for _, raw := range group.Suffixes {
			domain, err := normalizeDomain(raw)
			if err != nil {
				return fmt.Errorf("gateway: site group %q: %w", name, err)
			}
			if owner, exists := suffixOwners[domain]; exists && owner != name {
				return fmt.Errorf("gateway: suffix %q belongs to site groups %q and %q", domain, owner, name)
			}
			suffixOwners[domain] = name
		}
	}

	return nil
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
