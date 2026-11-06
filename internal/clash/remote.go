package clash

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/netsafe"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

const maxClashNodeBody = 8 << 20

type nodeEnvelope struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

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

func nodeURL(n *model.Node, path string) (string, error) {
	endpoint, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	addr, err := netsafe.NormalizeHost(n.Address)
	if err != nil {
		return "", err
	}
	scheme := n.Scheme
	if scheme != "http" && scheme != "https" {
		scheme = "https"
	}
	if n.Port <= 0 || n.Port > 65535 {
		return "", fmt.Errorf("invalid node port %d", n.Port)
	}
	bp := n.BasePath
	if bp == "" {
		bp = "/"
	}
	if !strings.HasPrefix(bp, "/") {
		bp = "/" + bp
	}
	if !strings.HasSuffix(bp, "/") {
		bp += "/"
	}
	u := url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort(addr, strconv.Itoa(n.Port)),
		Path:     bp + strings.TrimPrefix(endpoint.Path, "/"),
		RawQuery: endpoint.RawQuery,
	}
	return u.String(), nil
}

func nodeHTTPClient(n *model.Node) (*http.Client, error) {
	proxyURL := ""
	if mgr := runtime.GetManager(); mgr != nil {
		proxyURL = mgr.NodeEgressProxyURL(n.Id)
	}
	client, err := runtime.HTTPClientForNode(n, proxyURL)
	if err != nil {
		return nil, err
	}
	cp := *client
	cp.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cp, nil
}

func nodeToken(n *model.Node) (string, error) {
	if n.ApiToken == "" {
		if n.TlsVerifyMode == "mtls" {
			return "", nil
		}
		return "", errors.New("node has no API token")
	}
	return nodetoken.Decrypt(n.Id, n.ApiToken)
}

func callNode(ctx context.Context, n *model.Node, method, path string, body any, out any) error {
	token, err := nodeToken(n)
	if err != nil {
		return err
	}
	target, err := nodeURL(n, path)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client, err := nodeHTTPClient(n)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxClashNodeBody+1))
	if err != nil {
		return err
	}
	if len(raw) > maxClashNodeBody {
		return errors.New("clash node response too large")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clash node HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var env nodeEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode clash node response: %w", err)
	}
	if !env.Success {
		msg := strings.TrimSpace(env.Msg)
		if msg == "" {
			msg = "node rejected the policy"
		}
		return errors.New(msg)
	}
	if out != nil && len(env.Obj) > 0 {
		if err := json.Unmarshal(env.Obj, out); err != nil {
			return fmt.Errorf("decode clash node object: %w", err)
		}
	}
	return nil
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
