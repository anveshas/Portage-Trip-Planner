package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientSearchesWithInr(t *testing.T) {
	var sawAuth, sawCurrency bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/security/oauth2/token":
			body, _ := io.ReadAll(r.Body)
			if string(body) == "" {
				t.Errorf("empty token body")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
		case "/v2/shopping/flight-offers":
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Errorf("auth %s", r.Header.Get("Authorization"))
			}
			sawAuth = true
			if r.URL.Query().Get("currencyCode") != "INR" || r.URL.Query().Get("originLocationCode") != "DEL" {
				t.Errorf("query %s", r.URL.RawQuery)
			}
			sawCurrency = true
			_, _ = w.Write([]byte(`{"data":[{"id":"1","price":{"currency":"INR","grandTotal":"1000.00"},"itineraries":[{"segments":[{"departure":{"iataCode":"DEL","at":"2026-12-10T08:00:00"},"arrival":{"iataCode":"JAI","at":"2026-12-10T09:10:00"},"carrierCode":"6E","number":"201"}]}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &Client{Host: server.URL, ID: "id", Secret: "secret", HTTP: server.Client()}
	offers, err := client.Search(context.Background(), Query{Origin: "DEL", Destination: "JAI", Date: "2026-12-10", Travelers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !sawAuth || !sawCurrency {
		t.Fatalf("auth %v currency %v", sawAuth, sawCurrency)
	}
	if len(offers) != 1 || offers[0].Total != 1000 || offers[0].Source != "live" {
		t.Fatalf("%+v", offers)
	}
}

func TestClientTimeoutEmptyAndForeignCurrency(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/security/oauth2/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
			return
		}
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer slow.Close()
	client := &Client{Host: slow.URL, ID: "id", Secret: "secret", HTTP: &http.Client{Timeout: 50 * time.Millisecond}}
	if _, err := client.Search(context.Background(), Query{Origin: "DEL", Destination: "GOI", Date: "2026-12-10", Travelers: 1}); err == nil {
		t.Fatal("expected a timeout")
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/security/oauth2/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer empty.Close()
	client = &Client{Host: empty.URL, ID: "id", Secret: "secret", HTTP: empty.Client()}
	offers, err := client.Search(context.Background(), Query{Origin: "DEL", Destination: "GOI", Date: "2026-12-10", Travelers: 1})
	if err != nil || len(offers) != 0 {
		t.Fatalf("offers %+v err %v", offers, err)
	}

	usd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/security/oauth2/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-token"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"1","price":{"currency":"USD","grandTotal":"10.00"},"itineraries":[{"segments":[{"departure":{"iataCode":"DEL","at":"2026-12-10T08:00:00"},"arrival":{"iataCode":"JAI","at":"2026-12-10T09:10:00"},"carrierCode":"6E","number":"9"}]}]}]}`))
	}))
	defer usd.Close()
	client = &Client{Host: usd.URL, ID: "id", Secret: "secret", HTTP: usd.Client()}
	offers, err = client.Search(context.Background(), Query{Origin: "DEL", Destination: "JAI", Date: "2026-12-10", Travelers: 1})
	if err != nil || len(offers) != 0 {
		t.Fatalf("unconverted usd %+v %v", offers, err)
	}
	client.Rates = map[string]float64{"USD": 83}
	offers, err = client.Search(context.Background(), Query{Origin: "DEL", Destination: "JAI", Date: "2026-12-10", Travelers: 1})
	if err != nil || len(offers) != 1 || offers[0].Total != 830 {
		t.Fatalf("converted %+v %v", offers, err)
	}
}

func TestDisabledWithoutCredentials(t *testing.T) {
	t.Setenv("AMADEUS_CLIENT_ID", "")
	t.Setenv("AMADEUS_CLIENT_SECRET", "")
	flights := NewFromEnv(nil)
	if flights.Enabled() {
		t.Fatal("expected the sample path")
	}
}
