package runtime

import (
	"context"
	"encoding/json"
	"net/http"
)

type remoteRequestPolicy struct {
	maxResponseBytes int64
	rejectRedirects  bool
}

func (p remoteRequestPolicy) client(client *http.Client) *http.Client {
	if !p.rejectRedirects {
		return client
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}

// Project policy RPCs reuse upstream authentication and transport with a smaller body cap.
func (r *Remote) CallProject(ctx context.Context, method, path string, body, out any) error {
	env, err := r.request(ctx, method, path, body, remoteRequestPolicy{maxResponseBytes: 8 << 20, rejectRedirects: true})
	if err != nil {
		return err
	}
	if out != nil && len(env.Obj) > 0 {
		return json.Unmarshal(env.Obj, out)
	}
	return nil
}
