package plan

import (
	"strings"
	"time"
)

const (
	liveQuoteTTL   = 15 * time.Minute
	sampleQuoteTTL = 24 * time.Hour
)

func quoteMeta(source string, retrieved time.Time) (currency, status, expires string) {
	currency = "INR"
	switch source {
	case "live":
		return currency, "live_test", retrieved.Add(liveQuoteTTL).Format(time.RFC3339)
	case "sample":
		return currency, "sample", retrieved.Add(sampleQuoteTTL).Format(time.RFC3339)
	case "estimated":
		return currency, "estimated", ""
	case "calculated":
		return currency, "calculated", ""
	case "manual":
		return currency, "manual", ""
	default:
		return currency, source, ""
	}
}

func describe(kind, source string) (providerName, quote string) {
	switch source {
	case "live":
		return "Amadeus test", "Amadeus test search result, not a guaranteed fare and not commercial availability"
	case "estimated":
		return "Portage cost rules", "estimate, not a receipt"
	case "calculated":
		return "Portage cost rules", "calculated from the booked lines"
	default:
		if kind == "hotel" {
			return "Portage sample inventory", "sample rate, not a confirmed reservation"
		}
		return "Portage sample inventory", "sample fare, not a confirmed ticket"
	}
}

func transportLine(leg Leg) Line {
	return Line{
		Kind: leg.Mode, Label: leg.Carrier + " " + leg.ID + " " + leg.FromName + " to " + leg.ToName,
		Amount: leg.Total, Source: leg.Source, Counted: true,
	}
}

func finish(result *Result, retrieved time.Time) {
	stamp := retrieved.Format(time.RFC3339)
	result.RetrievedAt = stamp
	result.Booked = true
	if result.RequestedTotal == 0 {
		result.RequestedTotal = result.Total
	}
	if result.MinimumBudget == 0 {
		result.MinimumBudget = result.Total
	}
	if result.Reductions == nil {
		result.Reductions = []Reduction{}
	}
	sampleRide := false
	liveRide := false
	for i := range result.Legs {
		providerName, quote := describe(result.Legs[i].Mode, result.Legs[i].Source)
		result.Legs[i].Provider = providerName
		result.Legs[i].Quote = quote
		result.Legs[i].RetrievedAt = stamp
		result.Legs[i].Currency, result.Legs[i].DataStatus, result.Legs[i].ExpiresAt = quoteMeta(result.Legs[i].Source, retrieved)
		if result.Legs[i].Source == "sample" {
			sampleRide = true
		}
		if result.Legs[i].Source == "live" {
			liveRide = true
		}
	}
	for i := range result.Stays {
		result.Stays[i].Provider, result.Stays[i].Quote = describe("hotel", result.Stays[i].Source)
		result.Stays[i].RetrievedAt = stamp
		result.Stays[i].Currency, result.Stays[i].DataStatus, result.Stays[i].ExpiresAt = quoteMeta(result.Stays[i].Source, retrieved)
	}
	for i := range result.Lines {
		result.Lines[i].Provider, result.Lines[i].Quote = describe(result.Lines[i].Kind, result.Lines[i].Source)
		result.Lines[i].RetrievedAt = stamp
		result.Lines[i].Currency, result.Lines[i].DataStatus, result.Lines[i].ExpiresAt = quoteMeta(result.Lines[i].Source, retrieved)
	}
	if result.Recovery != nil {
		for i := range result.Recovery.Suggestions {
			if result.Recovery.Suggestions[i].Total == 0 {
				result.Recovery.Suggestions[i].Total = result.Total
				result.Recovery.Suggestions[i].Fits = result.FitsBudget
			}
		}
	}
	var notes []string
	if sampleRide {
		notes = append(notes, "Rides marked sample are from the Portage development inventory, retrieved at "+stamp+". They are not confirmed tickets.")
	}
	if liveRide {
		notes = append(notes, "Rides marked live are Amadeus test search results, retrieved at "+stamp+". A test search is not commercial availability and the fare can change before booking.")
	}
	result.FareNotice = strings.Join(notes, " ")
}
