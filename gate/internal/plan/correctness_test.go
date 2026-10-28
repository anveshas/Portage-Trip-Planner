package plan

import (
	"context"
	"strings"
	"testing"
	"time"

	"portage/gate/internal/provider"
)

func TestRankingKeepsThePrimaryDestination(t *testing.T) {
	req := demoRequest()
	longerEarlier := Result{Stays: []Stay{{City: "JAI", Nights: 2}}, Total: 18935}
	shorterPrimary := Result{Stays: []Stay{{City: "BOM", Nights: 1}}, Total: 35891}
	if !betterFit(shorterPrimary, longerEarlier, req) {
		t.Fatal("Mumbai is the last requested city and must beat a longer Jaipur stay")
	}
	both := Result{Stays: []Stay{{City: "JAI", Nights: 1}, {City: "BOM", Nights: 1}}, Total: 40984}
	if !betterFit(both, shorterPrimary, req) {
		t.Fatal("a plan that keeps every requested city must beat the primary city alone")
	}
	if betterFit(longerEarlier, shorterPrimary, req) {
		t.Fatal("extra nights in an earlier city must not beat the primary destination")
	}
}

func TestBudgetModelDoesNotDoubleCount(t *testing.T) {
	result := buildDemo(t)
	if !strings.Contains(result.Rules.BudgetScope, "whole group") {
		t.Fatal(result.Rules.BudgetScope)
	}
	if !strings.Contains(result.Rules.HotelBasis, "per room") || !strings.Contains(result.Rules.CostOrder, "once") {
		t.Fatalf("basis %s order %s", result.Rules.HotelBasis, result.Rules.CostOrder)
	}
	if result.Rules.Ranking[0] != "most requested cities" || result.Rules.Ranking[1] != "keep the last requested city" {
		t.Fatal(result.Rules.Ranking)
	}
	subtotal := 0
	contingency := 0
	tax := 0
	for _, line := range result.Lines {
		if line.Kind == "contingency" {
			contingency = line.Amount
			continue
		}
		if line.Kind == "tax" {
			tax += line.Amount
		}
		if line.Counted {
			subtotal += line.Amount
		}
	}
	stayTax := 0
	for _, stay := range result.Stays {
		if stay.Rooms != (2+1)/2 {
			t.Fatalf("rooms %d", stay.Rooms)
		}
		if stay.RoomTotal != stay.Nightly*stay.Nights*stay.Rooms {
			t.Fatalf("room total %+v", stay)
		}
		if stay.Tax != roundHalfUp(stay.RoomTotal, stay.TaxRate) {
			t.Fatalf("tax %+v", stay)
		}
		stayTax += stay.Tax
	}
	if tax != stayTax {
		t.Fatalf("tax lines %d stays %d", tax, stayTax)
	}
	if contingency != roundHalfUp(subtotal, 8) || subtotal+contingency != result.Total {
		t.Fatalf("subtotal %d contingency %d total %d", subtotal, contingency, result.Total)
	}
	if sumLines(result) != result.Total {
		t.Fatalf("lines %d total %d", sumLines(result), result.Total)
	}
}

func TestTimingRejectsMissedOverlapAndImpossibleLegs(t *testing.T) {
	result := buildDemo(t)
	missed := result
	missed.Legs[1].Depart = missed.Legs[0].Arrive
	if !hasCode(Validate(missed), "connection_too_tight") {
		t.Fatal(Validate(missed))
	}
	overlap := result
	overlap.Stays[1].CheckIn = overlap.Stays[0].CheckIn
	if !hasCode(Validate(overlap), "overlap") && !hasCode(Validate(overlap), "date_conflict") {
		t.Fatal(Validate(overlap))
	}
	impossible := result
	impossible.Legs[0].Arrive = impossible.Legs[0].Depart
	if !hasCode(Validate(impossible), "impossible_timing") {
		t.Fatal(Validate(impossible))
	}
	shifted := result
	shifted.Legs[0].Arrive = "2026-12-12T18:00:00+05:30"
	shifted.Legs[1].Depart = "2026-12-12T13:00:00Z"
	shifted.Legs[1].Arrive = "2026-12-12T15:30:00Z"
	if !hasCode(Validate(shifted), "connection_too_tight") {
		t.Fatal(Validate(shifted))
	}
}

func TestExpiredLiveQuoteIsRevalidated(t *testing.T) {
	result := buildDemo(t)
	past := time.Now().Add(-time.Hour)
	result.Legs[0].Source = "live"
	result.Legs[0].DataStatus = "live_test"
	result.Legs[0].ExpiresAt = past.Format(time.RFC3339)
	result.Lines[0].Source = "live"
	result.Lines[0].DataStatus = "live_test"
	result.Lines[0].ExpiresAt = result.Legs[0].ExpiresAt
	MarkFreshness(&result, time.Now())
	if result.Legs[0].DataStatus != "expired" || result.Valid {
		t.Fatalf("status %s valid %v issues %+v", result.Legs[0].DataStatus, result.Valid, result.Issues)
	}
	empty, err := Revalidate(context.Background(), &fakeFlights{}, result, 2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if empty.Legs[0].DataStatus != "unavailable" || empty.Legs[0].Total == 0 || empty.Valid {
		t.Fatalf("leg %+v valid %v", empty.Legs[0], empty.Valid)
	}
	depart := time.Date(2026, 12, 10, 8, 0, 0, 0, ist)
	arrive := time.Date(2026, 12, 10, 9, 10, 0, 0, ist)
	flights := &fakeFlights{offers: []provider.Offer{{
		ID: "6E777", Carrier: "IndiGo", Origin: "DEL", Destination: "JAI",
		Depart: depart, Arrive: arrive, Total: 1000, Source: "live",
	}}}
	fresh, err := Revalidate(context.Background(), flights, result, 2, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Legs[0].ID != "6E777" || fresh.Legs[0].DataStatus != "live_test" || fresh.Legs[0].Currency != "INR" {
		t.Fatalf("leg %+v", fresh.Legs[0])
	}
	if !strings.Contains(fresh.Legs[0].Quote, "not commercial availability") || fresh.Total != 49784 || !fresh.Valid {
		t.Fatalf("total %d valid %v quote %s issues %+v", fresh.Total, fresh.Valid, fresh.Legs[0].Quote, fresh.Issues)
	}
	if fresh.Legs[1].DataStatus != "sample" {
		t.Fatalf("sample leg became %s", fresh.Legs[1].DataStatus)
	}
}

func TestSampleQuoteCarriesCurrencyAndStatus(t *testing.T) {
	result := buildDemo(t)
	if result.Legs[0].Currency != "INR" || result.Legs[0].DataStatus != "sample" || result.Legs[0].ExpiresAt == "" {
		t.Fatalf("%+v", result.Legs[0])
	}
	if result.Stays[0].DataStatus != "sample" || result.Lines[len(result.Lines)-1].DataStatus != "calculated" {
		t.Fatalf("stay %s last line %s", result.Stays[0].DataStatus, result.Lines[len(result.Lines)-1].DataStatus)
	}
}

func hasCode(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
