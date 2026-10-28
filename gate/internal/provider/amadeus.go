package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	Host   string
	ID     string
	Secret string
	HTTP   *http.Client
	Rates  map[string]float64
}

func NewFromEnv(client *http.Client) Flights {
	id := os.Getenv("AMADEUS_CLIENT_ID")
	secret := os.Getenv("AMADEUS_CLIENT_SECRET")
	if id == "" || secret == "" {
		return Disabled{}
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	host := os.Getenv("AMADEUS_HOST")
	if host == "" {
		host = "https://test.api.amadeus.com"
	}
	rates := map[string]float64{}
	if raw := os.Getenv("FX_USD_INR"); raw != "" {
		rate, err := strconv.ParseFloat(raw, 64)
		if err == nil && rate > 0 {
			rates["USD"] = rate
		}
	}
	return &Client{Host: strings.TrimRight(host, "/"), ID: id, Secret: secret, HTTP: client, Rates: rates}
}

func (c *Client) Enabled() bool { return true }

func (c *Client) Search(ctx context.Context, query Query) ([]Offer, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("originLocationCode", query.Origin)
	values.Set("destinationLocationCode", query.Destination)
	values.Set("departureDate", query.Date)
	values.Set("adults", strconv.Itoa(query.Travelers))
	values.Set("currencyCode", "INR")
	values.Set("max", "10")
	endpoint := c.Host + "/v2/shopping/flight-offers?" + values.Encode()
	body, err := c.get(ctx, endpoint, token)
	if err != nil {
		return nil, err
	}
	offers, skips, err := Normalize(body, c.Rates)
	for _, skip := range skips {
		log.Printf("amadeus skip id=%s reason=%s", skip.ID, skip.Reason)
	}
	return offers, err
}

func (c *Client) token(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.ID)
	form.Set("client_secret", c.Secret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Host+"/v1/security/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("amadeus token returned %d", response.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", err
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("amadeus token was empty")
	}
	return parsed.AccessToken, nil
}

func (c *Client) get(ctx context.Context, endpoint, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("amadeus search returned %d", response.StatusCode)
	}
	return payload, nil
}
