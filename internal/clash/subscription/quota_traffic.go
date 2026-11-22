package subscription

import (
	"github.com/mhsanaei/3x-ui/v3/internal/clash/allocation"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// NodeQuotaTraffic reads the same per-node ledgers as the client editor.
func NodeQuotaTraffic(emails []string) (*xray.ClientTraffic, []string, error) {
	if policy.GetRuntime() == nil || len(emails) == 0 {
		return nil, emails, nil
	}
	db, err := storage.DB()
	if err != nil {
		return nil, nil, err
	}
	var ids []int
	if err = db.Model(&allocation.ClientRatio{}).Joins("JOIN clients ON clients.id = clash_client_ratios.client_id").Where("clients.email IN ?", emails).Pluck("clients.id", &ids).Error; err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return nil, emails, nil
	}
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		return nil, nil, err
	}
	covered := map[string]bool{}
	var traffic xray.ClientTraffic
	unlimited := false
	for _, id := range ids {
		view, err := policy.LoadClientView(cat, id)
		if err != nil {
			return nil, nil, err
		}
		if view.ManagedTotalBytes == nil {
			continue
		}
		covered[view.Email] = true
		unlimited = unlimited || *view.ManagedTotalBytes == 0
		traffic.Total += *view.ManagedTotalBytes
		for _, node := range view.Nodes {
			traffic.Up += node.UsageUp
			traffic.Down += node.UsageDown
		}
	}
	if len(covered) == 0 {
		return nil, emails, nil
	}
	if unlimited {
		traffic.Total = 0
	}
	var plain []string
	for _, email := range emails {
		if !covered[email] {
			plain = append(plain, email)
		}
	}
	return &traffic, plain, nil
}
