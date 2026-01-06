package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Client wraps HTTP calls to the OL2 API.
type Client struct {
	cfg    *Config
	client *http.Client
}

// NewClient creates a new Client with sane defaults.
func NewClient(cfg *Config) *Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	return &Client{
		cfg:    cfg,
		client: httpClient,
	}
}

// APIResponse is the top-level response from OL2 API.
type APIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		RosterList []RosterItem `json:"rosterList"`
		Limit      struct {
			Fav struct {
				Max     int `json:"max"`
				Current int `json:"current"`
			} `json:"fav"`
		} `json:"limit"`
	} `json:"data"`
}

// RosterItem represents a single player entry.
type RosterItem struct {
	PlayerID string `json:"playerId"`
	Grade    string `json:"grade"`

	ShowName    string `json:"showName"`
	PlayerEn    string `json:"PlayerEnName"`
	PlayerImg   string `json:"playerImg"`
	TeamAbbr    string `json:"teamAbbr"`
	CardTypeStr string `json:"cardType"`
	VersionStr  string `json:"Version"`

	Price struct {
		StandardPrice       int    `json:"standardPrice"`
		CurrentLowestPrice  string `json:"currentLowestPrice"`
		LowerPriceForSale   int    `json:"lowerPriceForSale"`
		UpperPriceForSale   int    `json:"upperPriceForSale"`
		Popularity          string `json:"popularity"`
		SalePrice           int    `json:"salePrice"`
	} `json:"price"`
}

// FetchRoster fetches current roster data from OL2 API.
func (c *Client) FetchRoster() (*APIResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ts := time.Now().UnixMilli()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.cfg.BaseURL,
		nil,
	)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("openid", c.cfg.OpenID)
	q.Set("access_token", c.cfg.AccessToken)
	q.Set("orderBy", "")
	q.Set("orientation", "")
	q.Set("login_channel", c.cfg.LoginChannel)
	q.Set("timeStamp", fmt.Sprintf("%d", ts))
	q.Set("nonseStr", c.cfg.NonseStr)
	q.Set("sign", c.cfg.Sign)
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var out APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	if out.Code != 0 {
		return nil, fmt.Errorf("api error: code=%d msg=%s", out.Code, out.Msg)
	}

	return &out, nil
}


