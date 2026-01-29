package weheat

import (
	"context"
	"fmt"

	weheatapi "github.com/joshp123/weheat-golang"

	"github.com/joshp123/gohome/internal/oauth"
	configv1 "github.com/joshp123/gohome/proto/gen/config/v1"
)

// Client wraps the weheat-golang client with GoHome OAuth handling.
type Client struct {
	api   *weheatapi.Client
	oauth *oauth.Manager
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

	opts := []weheatapi.ClientOption{weheatapi.WithTokenSource(oauthTokenSource{manager: manager})}
	if cfg.BaseURL != "" {
		opts = append(opts, weheatapi.WithBaseURL(cfg.BaseURL))
	}

	api, err := weheatapi.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	return &Client{api: api, oauth: manager}, nil
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

func (c *Client) LatestLog(ctx context.Context, id string) (*weheatapi.RawHeatPumpLog, error) {
	return c.api.GetLatestLog(ctx, id, weheatapi.RequestOptions{})
}

func (c *Client) RawLogs(ctx context.Context, id string, query weheatapi.LogQuery) ([]weheatapi.RawHeatPumpLog, error) {
	return c.api.GetRawLogs(ctx, id, query)
}

func (c *Client) LogViews(ctx context.Context, id string, query weheatapi.LogQuery) ([]weheatapi.HeatPumpLogView, error) {
	return c.api.GetLogs(ctx, id, query)
}

func (c *Client) EnergyTotals(ctx context.Context, id string) (*weheatapi.TotalEnergyAggregate, error) {
	return c.api.GetEnergyTotals(ctx, id, weheatapi.RequestOptions{})
}

func (c *Client) EnergyLogs(ctx context.Context, id string, query weheatapi.EnergyLogQuery) ([]weheatapi.EnergyView, error) {
	return c.api.GetEnergyLogs(ctx, id, query)
}

func (c *Client) DiscoverActiveHeatPumps(ctx context.Context) ([]weheatapi.HeatPumpInfo, error) {
	return c.api.DiscoverActiveHeatPumps(ctx)
}
