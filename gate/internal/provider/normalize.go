package provider

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

var ist = time.FixedZone("IST", 5*3600+30*60)

type Skip struct {
	ID     string
	Reason string
}

// Normalize reads an Amadeus flight-offers payload.
// INR totals are kept. Another currency is converted only when rates supplies a positive INR-per-unit rate.
// A fractional rupee rounds up so the plan never understates the fare.
// grandTotal already includes fees, so fees are recorded and not added again.
func Normalize(body []byte, rates map[string]float64) ([]Offer, []Skip, error) {
	var payload struct {
		Data []struct {
			ID    string `json:"id"`
			Price struct {
				Currency   string `json:"currency"`
				GrandTotal string `json:"grandTotal"`
				Fees       []struct {
					Amount string `json:"amount"`
				} `json:"fees"`
			} `json:"price"`
			Itineraries []struct {
				Segments []struct {
					Departure struct {
						IATA string `json:"iataCode"`
						At   string `json:"at"`
					} `json:"departure"`
					Arrival struct {
						IATA string `json:"iataCode"`
						At   string `json:"at"`
					} `json:"arrival"`
					Carrier string `json:"carrierCode"`
					Number  string `json:"number"`
				} `json:"segments"`
			} `json:"itineraries"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}
	var offers []Offer
	var skips []Skip
	for _, item := range payload.Data {
		total, err := ToINR(item.Price.GrandTotal, item.Price.Currency, rates)
		if err != nil {
			skips = append(skips, Skip{ID: item.ID, Reason: err.Error()})
			continue
		}
		if len(item.Itineraries) == 0 || len(item.Itineraries[0].Segments) == 0 {
			skips = append(skips, Skip{ID: item.ID, Reason: "missing itinerary"})
			continue
		}
		segments := item.Itineraries[0].Segments
		first, last := segments[0], segments[len(segments)-1]
		depart, err := parseWhen(first.Departure.At)
		if err != nil {
			skips = append(skips, Skip{ID: item.ID, Reason: "missing time"})
			continue
		}
		arrive, err := parseWhen(last.Arrival.At)
		if err != nil {
			skips = append(skips, Skip{ID: item.ID, Reason: "missing time"})
			continue
		}
		fees := 0
		for _, fee := range item.Price.Fees {
			amount, feeErr := ceilRupees(fee.Amount)
			if feeErr == nil {
				fees += amount
			}
		}
		id := first.Carrier + first.Number
		if id == "" {
			id = item.ID
		}
		offers = append(offers, Offer{
			ID: id, Carrier: carrierName(first.Carrier), Origin: first.Departure.IATA, Destination: last.Arrival.IATA,
			Depart: depart, Arrive: arrive, Total: total, Fees: fees, Source: "live",
		})
	}
	return offers, skips, nil
}

// ToINR converts a decimal amount into whole rupees, rounding up.
// INR uses the amount itself. Any other currency needs rates[currency] as rupees per one unit.
func ToINR(raw, currency string, rates map[string]float64) (int, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return 0, fmt.Errorf("missing currency")
	}
	if currency == "INR" {
		return ceilRupees(raw)
	}
	rate := 0.0
	if rates != nil {
		rate = rates[currency]
	}
	if rate <= 0 {
		return 0, fmt.Errorf("%s was not converted because no rate is configured", currency)
	}
	hundredths, err := toHundredths(raw)
	if err != nil {
		return 0, err
	}
	rupees := int(math.Ceil(float64(hundredths) * rate / 100.0))
	if rupees <= 0 {
		return 0, fmt.Errorf("missing price")
	}
	return rupees, nil
}

func toHundredths(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") {
		return 0, fmt.Errorf("missing price")
	}
	wholeText, frac, hasFrac := strings.Cut(raw, ".")
	whole, err := strconv.Atoi(wholeText)
	if err != nil || whole < 0 {
		return 0, fmt.Errorf("missing price")
	}
	if !hasFrac {
		return whole * 100, nil
	}
	extra := false
	if len(frac) > 2 {
		extra = strings.Trim(frac[2:], "0") != ""
		frac = frac[:2]
	}
	for len(frac) < 2 {
		frac += "0"
	}
	cents, err := strconv.Atoi(frac)
	if err != nil {
		return 0, fmt.Errorf("missing price")
	}
	if extra {
		cents++
	}
	return whole*100 + cents, nil
}

func carrierName(code string) string {
	if code == "6E" {
		return "IndiGo"
	}
	if code == "" {
		return "Airline"
	}
	return code
}

func parseWhen(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("missing time")
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.ParseInLocation("2006-01-02T15:04:05", value, ist)
}

func ceilRupees(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") {
		return 0, fmt.Errorf("missing price")
	}
	wholeText, frac, hasFrac := strings.Cut(raw, ".")
	whole, err := strconv.Atoi(wholeText)
	if err != nil || whole < 0 {
		return 0, fmt.Errorf("missing price")
	}
	if !hasFrac || strings.Trim(frac, "0") == "" {
		if whole == 0 {
			return 0, fmt.Errorf("missing price")
		}
		return whole, nil
	}
	return whole + 1, nil
}
