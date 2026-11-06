package clash

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

// LoadClientView joins the client's node settings with each node's own ledger.
// Missing reports remain unknown; official aggregate traffic is not split or
// assigned to nodes by proportion.
func LoadClientView(cat *Catalog, clientID int) (ClientRatioView, error) {
	nodes, err := ListNodeWeights()
	if err != nil {
		return ClientRatioView{}, err
	}
	ratio, err := GetRatio(clientID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return ClientRatioView{}, err
	}
	view, err := BuildClientRatioView(cat, clientID, NodeConfigMap(cat, nodes), ratio)
	if err != nil {
		return view, err
	}
	ratios, err := ListRatios()
	if err != nil {
		return view, err
	}
	allocated := NodeAllocations(cat, RatioMap(ratios))
	for i := range view.Nodes {
		n := &view.Nodes[i]
		n.AllocatedBytes = allocated[n.NodeKey]
		d, err := GetDelivery(n.NodeKey, clientID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return view, err
		}
		n.ApplyError = d.LastError
		var report PolicyReport
		if n.NodeKey == LocalNodeKey && GetRuntime() != nil {
			report, err = GetRuntime().Report(cat.PanelGuid, clientID)
			if errors.Is(err, ErrPolicyNotFound) {
				continue
			}
			if err != nil {
				return view, err
			}
		} else {
			if d.LastReportRaw == "" {
				continue
			}
			if err := json.Unmarshal([]byte(d.LastReportRaw), &report); err != nil {
				return view, err
			}
		}
		n.PeriodEnd = report.PeriodEnd
		n.UsageKnown = true
		n.UsageUp, n.UsageDown = report.UsageUp, report.UsageDown
		n.State = report.State
		n.Applied = report.Applied && report.Revision == d.Revision && !d.Remove && n.ApplyError == ""
		if n.ApplyError == "" {
			n.ApplyError = report.ApplyError
		}
	}
	return view, nil
}
