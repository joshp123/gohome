package weheat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	weheatapi "github.com/joshp123/weheat-golang"

	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

// Client wraps the weheat-golang client with GoHome OAuth handling.
type Client struct {
	api         *weheatapi.Client
	baseURL     string
	tokenSource weheatapi.TokenSource
	oauth       *oauth.Manager
}

type oauthTokenSource struct {
	manager *oauth.Manager
}

func (o oauthTokenSource) Token(ctx context.Context) (string, error) {
	if o.manager == nil {
		return "", fmt.Errorf("oauth manager not configured")
	}
	return o.manager.AccessToken(ctx)
}

func NewClient(cfg Config, bootstrap oauth.Bootstrap, decl oauth.Declaration, oauthCfg *configv1.OAuthConfig) (*Client, error) {
	blobStore, err := oauth.NewS3Store(oauthCfg)
	if err != nil {
		return nil, err
	}

	manager, err := oauth.NewManagerFromBootstrap(decl, bootstrap, blobStore)
	if err != nil {
		return nil, err
	}
	manager.StartWithInterval(context.Background(), oauth.RefreshInterval(oauthCfg))

	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	opts := []weheatapi.ClientOption{
		weheatapi.WithTokenSource(oauthTokenSource{manager: manager}),
		weheatapi.WithHTTPClient(&http.Client{Transport: apiPathTransport{prefix: strings.TrimRight(base.Path, "/")}}),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, weheatapi.WithBaseURL(cfg.BaseURL))
	}

	api, err := weheatapi.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &Client{api: api, oauth: manager, baseURL: cfg.BaseURL, tokenSource: oauthTokenSource{manager: manager}}, nil
}

func (c *Client) ListHeatPumps(ctx context.Context, state *weheatapi.DeviceState) ([]weheatapi.ReadAllHeatPump, error) {
	resp, err := c.api.ListHeatPumps(ctx, weheatapi.ListHeatPumpsParams{State: state})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return resp.Data, nil
}

func (c *Client) HeatPump(ctx context.Context, id string) (*weheatapi.ReadHeatPump, error) {
	return c.api.GetHeatPump(ctx, id, weheatapi.RequestOptions{})
}

func (c *Client) LatestLog(ctx context.Context, id string) (*latestLog, error) {
	var out latestLog
	err := c.readJSON(ctx, "/api/v1/heat-pumps/"+url.PathEscape(id)+"/logs/latest", nil, &out)
	return &out, err
}

func (c *Client) RawLogs(ctx context.Context, id string, query weheatapi.LogQuery) ([]weheatapi.RawHeatPumpLog, error) {
	return c.api.GetRawLogs(ctx, id, query)
}

func (c *Client) LogViews(ctx context.Context, id string, query weheatapi.LogQuery) ([]json.RawMessage, error) {
	values := url.Values{"interval": {string(query.Interval)}}
	if query.StartTime != nil {
		values.Set("startTime", query.StartTime.UTC().Format(time.RFC3339))
	}
	if query.EndTime != nil {
		values.Set("endTime", query.EndTime.UTC().Format(time.RFC3339))
	}
	var out []json.RawMessage
	err := c.readJSON(ctx, "/api/v1/heat-pumps/"+url.PathEscape(id)+"/logs", values, &out)
	return out, err
}

func (c *Client) EnergyTotals(ctx context.Context, id string) (*energyTotals, error) {
	var out energyTotals
	err := c.readJSON(ctx, "/api/v1/energy-logs/"+url.PathEscape(id)+"/total", nil, &out)
	return &out, err
}

func (c *Client) EnergyLogs(ctx context.Context, id string, query weheatapi.EnergyLogQuery) ([]energyLog, error) {
	values := url.Values{"interval": {string(query.Interval)}}
	if query.StartTime != nil {
		values.Set("startTime", query.StartTime.UTC().Format(time.RFC3339))
	}
	if query.EndTime != nil {
		values.Set("endTime", query.EndTime.UTC().Format(time.RFC3339))
	}
	var out []energyLog
	err := c.readJSON(ctx, "/api/v1/energy-logs/"+url.PathEscape(id), values, &out)
	return out, err
}

func (c *Client) DiscoverActiveHeatPumps(ctx context.Context) ([]weheatapi.HeatPumpInfo, error) {
	return c.api.DiscoverActiveHeatPumps(ctx)
}

// The upstream client resolves absolute endpoint paths, discarding the base URL path.
// Preserve the documented third-party prefix until upstream fixes URL resolution.
type apiPathTransport struct{ prefix string }

func (t apiPathTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	request.URL.Path = t.prefix + request.URL.Path
	return http.DefaultTransport.RoundTrip(request)
}

// Decode source responses without the dependency dropping newer fields or
// turning absent observations into fabricated zeroes.
func (c *Client) readJSON(ctx context.Context, path string, query url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = query.Encode()
	if c.tokenSource != nil {
		token, err := c.tokenSource.Token(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Weheat read %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
