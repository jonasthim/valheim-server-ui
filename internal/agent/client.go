package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// maxResponseBytes bounds one agent response (a few hundred players at most).
const maxResponseBytes = 4 << 20

// Client talks to one instance's agent over loopback.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient returns a client for the agent listening on port (the instance's
// game port) with the token written by EnsureConfig.
func NewClient(port int, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Second}
	}
	return &Client{base: fmt.Sprintf("http://%s:%d", defaultBind, port), token: token, http: hc}
}

// NewClientForURL is NewClient for an explicit base URL (tests).
func NewClientForURL(base, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Second}
	}
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: hc}
}

// Status fetches the world snapshot.
func (c *Client) Status(ctx context.Context) (*domain.AgentStatus, error) {
	var st domain.AgentStatus
	if err := c.do(ctx, http.MethodGet, "/v1/status", nil, "", &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// Events returns the agent's events after seq and the newest sequence number.
func (c *Client) Events(ctx context.Context, since int64) ([]domain.AgentEvent, int64, error) {
	var out struct {
		Next   int64               `json:"next"`
		Events []domain.AgentEvent `json:"events"`
	}
	q := url.Values{"since": {fmt.Sprint(since)}}
	if err := c.do(ctx, http.MethodGet, "/v1/events", q, "", &out); err != nil {
		return nil, 0, err
	}
	return out.Events, out.Next, nil
}

// Command runs one of domain.AgentCommands. A refusal by the agent (bad
// target, empty message) comes back as OK=false with its message, not as an
// error; errors are transport or protocol failures.
func (c *Client) Command(ctx context.Context, req domain.AgentCommandRequest) (*domain.AgentCommandResult, error) {
	q := url.Values{}
	if req.Target != "" {
		q.Set("target", req.Target)
	}
	if req.Style != "" {
		q.Set("style", req.Style)
	}
	var res domain.AgentCommandResult
	err := c.do(ctx, http.MethodPost, "/v1/commands/"+url.PathEscape(req.Command), q, req.Message, &res)
	var se *statusError
	if err != nil {
		if asStatusError(err, &se) && se.code == http.StatusBadRequest && res.Message != "" {
			return &res, nil
		}
		return nil, err
	}
	return &res, nil
}

type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string { return fmt.Sprintf("agent: HTTP %d: %s", e.code, e.body) }

func asStatusError(err error, target **statusError) bool {
	se, ok := err.(*statusError)
	if ok {
		*target = se
	}
	return ok
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body string, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return fmt.Errorf("agent: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("agent: read response: %w", err)
	}
	if out != nil && len(data) > 0 && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest) {
		_ = json.Unmarshal(data, out)
	}
	if resp.StatusCode != http.StatusOK {
		return &statusError{code: resp.StatusCode, body: strings.TrimSpace(string(data))}
	}
	return nil
}

// MapInfo fetches the plugin's render state.
func (c *Client) MapInfo(ctx context.Context) (*domain.MapInfo, error) {
	var info domain.MapInfo
	if err := c.do(ctx, http.MethodGet, "/v1/map/info", nil, "", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// MapObjects fetches the points of interest.
func (c *Client) MapObjects(ctx context.Context) (*domain.MapObjects, error) {
	var out domain.MapObjects
	if err := c.do(ctx, http.MethodGet, "/v1/map/objects", nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MapPNG fetches the rendered map. While the plugin is still rendering it
// returns (nil, info, nil) with the render progress.
func (c *Client) MapPNG(ctx context.Context) ([]byte, *domain.MapInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/map", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMapBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("agent: read map: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if len(data) < 8 || string(data[1:4]) != "PNG" {
			return nil, nil, &statusError{code: resp.StatusCode, body: "map response is not a PNG"}
		}
		return data, nil, nil
	case http.StatusAccepted:
		var info domain.MapInfo
		_ = json.Unmarshal(data, &info)
		return nil, &info, nil
	default:
		return nil, nil, &statusError{code: resp.StatusCode, body: strings.TrimSpace(string(data))}
	}
}

// RenderMap asks the plugin to (re)render at size (0 = its configured size).
func (c *Client) RenderMap(ctx context.Context, size int, force bool) (*domain.MapInfo, error) {
	q := url.Values{}
	if size > 0 {
		q.Set("size", fmt.Sprint(size))
	}
	if force {
		q.Set("force", "true")
	}
	var info domain.MapInfo
	err := c.do(ctx, http.MethodPost, "/v1/map/render", q, "", &info)
	var se *statusError
	if err != nil && asStatusError(err, &se) && se.code == http.StatusAccepted {
		_ = json.Unmarshal([]byte(se.body), &info)
		return &info, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// maxMapBytes bounds a map image (4096² PNG stays far below this).
const maxMapBytes = 64 << 20
