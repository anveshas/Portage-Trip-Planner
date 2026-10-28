package plan

import (
	"context"
	"strings"
	"time"

	"portage/gate/internal/provider"
)

func MarkFreshness(result *Result, now time.Time) {
	for i := range result.Legs {
		markQuote(&result.Legs[i].DataStatus, &result.Legs[i].Quote, result.Legs[i].ExpiresAt, now)
	}
	for i := range result.Stays {
		markQuote(&result.Stays[i].DataStatus, &result.Stays[i].Quote, result.Stays[i].ExpiresAt, now)
	}
	for i := range result.Lines {
		markQuote(&result.Lines[i].DataStatus, &result.Lines[i].Quote, result.Lines[i].ExpiresAt, now)
	}
	result.Issues = Validate(*result)
	result.Valid = len(result.Issues) == 0
	result.Report.ValidationPassed = result.Valid
}

func markQuote(status, quote *string, expires string, now time.Time) {
	if *status != "live_test" && *status != "sample" {
		return
	}
	exp, err := time.Parse(time.RFC3339, expires)
	if err != nil || now.Before(exp) {
		return
	}
	*status = "expired"
	*quote = "expired quote; the price can no longer be used without a new search"
}

// Revalidate searches again for expired live legs. Sample quotes stay sample.
// An empty new search is marked unavailable and is not treated as a free ride.
func Revalidate(ctx context.Context, flights provider.Flights, result Result, travelers int, now time.Time) (Result, error) {
	result.Legs = append([]Leg(nil), result.Legs...)
	result.Stays = append([]Stay(nil), result.Stays...)
	result.Lines = append([]Line(nil), result.Lines...)
	MarkFreshness(&result, now)
	if flights == nil || !flights.Enabled() {
		return result, nil
	}
	for i := range result.Legs {
		leg := &result.Legs[i]
		if leg.Source != "live" || leg.DataStatus != "expired" {
			continue
		}
		offers, err := flights.Search(ctx, provider.Query{
			Origin: leg.From, Destination: leg.To, Date: leg.Date, Travelers: travelers,
		})
		if err != nil {
			return result, err
		}
		offer, ok := cheapest(offers)
		if !ok {
			leg.DataStatus = "unavailable"
			leg.Quote = "expired, and the new search returned no bookable INR fare"
			stampMatchingLine(&result, leg.ID, *leg)
			continue
		}
		oldID := leg.ID
		leg.ID = offer.ID
		leg.Carrier = offer.Carrier
		leg.Total = offer.Total
		if travelers > 0 {
			leg.PerPerson = offer.Total / travelers
		}
		leg.Depart = offer.Depart.Format(time.RFC3339)
		leg.Arrive = offer.Arrive.Format(time.RFC3339)
		leg.RetrievedAt = now.Format(time.RFC3339)
		leg.Currency, leg.DataStatus, leg.ExpiresAt = quoteMeta("live", now)
		leg.Provider, leg.Quote = describe(leg.Mode, "live")
		stampMatchingLine(&result, oldID, *leg)
	}
	recompute(&result)
	return result, nil
}

func cheapest(offers []provider.Offer) (provider.Offer, bool) {
	if len(offers) == 0 {
		return provider.Offer{}, false
	}
	best := offers[0]
	for _, offer := range offers[1:] {
		if offer.Total < best.Total {
			best = offer
		}
	}
	return best, true
}

func stampMatchingLine(result *Result, oldID string, leg Leg) {
	for i := range result.Lines {
		line := &result.Lines[i]
		if line.Kind != leg.Mode || !strings.Contains(line.Label, oldID) {
			continue
		}
		line.Label = leg.Carrier + " " + leg.ID + " " + leg.FromName + " to " + leg.ToName
		line.Amount = leg.Total
		line.Source = leg.Source
		line.Provider = leg.Provider
		line.RetrievedAt = leg.RetrievedAt
		line.ExpiresAt = leg.ExpiresAt
		line.Currency = leg.Currency
		line.DataStatus = leg.DataStatus
		line.Quote = leg.Quote
		return
	}
}

func recompute(result *Result) {
	subtotal := 0
	contingencyAt := -1
	for i, line := range result.Lines {
		if line.Kind == "contingency" {
			contingencyAt = i
			continue
		}
		if line.Counted {
			subtotal += line.Amount
		}
	}
	extra := 0
	if contingencyAt >= 0 {
		extra = roundHalfUp(subtotal, contingencyPercent)
		result.Lines[contingencyAt].Amount = extra
	}
	result.Total = subtotal + extra
	result.FitsBudget = result.Total <= result.Budget
	result.Issues = Validate(*result)
	result.Valid = len(result.Issues) == 0
	result.Report.ValidationPassed = result.Valid
	result.Report.FitsBudget = result.FitsBudget
}
