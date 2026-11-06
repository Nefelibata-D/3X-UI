package clash

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

func loadNodeForDelivery(nodeKey string) (*model.Node, error) {
	if !strings.HasPrefix(nodeKey, "node:") {
		return nil, fmt.Errorf("not a remote node key: %s", nodeKey)
	}
	id, err := strconv.Atoi(strings.TrimPrefix(nodeKey, "node:"))
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid node key: %s", nodeKey)
	}
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	n := &model.Node{}
	if err := db.Where("id = ?", id).First(n).Error; err != nil {
		return nil, err
	}
	return n, nil
}

func callNode(ctx context.Context, n *model.Node, method, path string, body any, out any) error {
	var resolver runtime.NodeEgressResolver
	if mgr := runtime.GetManager(); mgr != nil {
		resolver = mgr
	}
	return runtime.NewRemote(n, resolver).CallProject(ctx, method, path, body, out)
}

func applyNode(ctx context.Context, n *model.Node, req ApplyRequest) (PolicyReport, error) {
	var report PolicyReport
	err := callNode(ctx, n, http.MethodPost, "panel/api/clash/policy/apply", req, &report)
	return report, err
}

func reportNode(ctx context.Context, n *model.Node, masterGuid string, clientID int) (PolicyReport, error) {
	var report PolicyReport
	path := "panel/api/clash/policy/report?masterGuid=" + url.QueryEscape(masterGuid) +
		"&clientId=" + strconv.Itoa(clientID)
	err := callNode(ctx, n, http.MethodGet, path, nil, &report)
	return report, err
}

func isStaleError(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrStaleRevision.Error())
}
