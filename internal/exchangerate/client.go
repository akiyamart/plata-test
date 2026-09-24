package exchangerate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"quoteservice/internal/domain"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		base = "https://api.exchangerate.dev"
	}
	return &Client{
		baseURL: base,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type rateResponse struct {
	Result        string          `json:"result"`
	Base          string          `json:"base"`
	Quote         string          `json:"quote"`
	Rate          decimal.Decimal `json:"rate"`
	DataUpdatedAt time.Time       `json:"data_updated_at"`
	Code          string          `json:"code"`
	Message       string          `json:"message"`
}

func (c *Client) Fetch(ctx context.Context, p domain.CurrencyPair) (domain.Quote, error) {
	url := fmt.Sprintf("%s/v1/rate/%s", c.baseURL, p.Slug())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.Quote{}, wrapErr(0, 0, err)
	}
	req.Header.Set("User-Agent", "quoteservice/1.0")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Quote{}, wrapErr(0, 0, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return domain.Quote{}, wrapErr(resp.StatusCode, 0, err)
	}
	var parsed rateResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return domain.Quote{}, wrapErr(resp.StatusCode, 0, fmt.Errorf("decode fx response: %w", err))
	}
	if resp.StatusCode != http.StatusOK || parsed.Result != "success" {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return domain.Quote{}, wrapErr(resp.StatusCode, retryAfter, fmtProviderErr(resp.StatusCode, parsed.Code))
	}
	q, err := domain.NewQuote(p, parsed.Rate, parsed.DataUpdatedAt.UTC())
	if err != nil {
		return domain.Quote{}, err
	}
	return q, nil
}
