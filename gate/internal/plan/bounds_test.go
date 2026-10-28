package plan

import (
	"context"
	"strings"
	"testing"

	"portage/gate/internal/catalog"
	"portage/gate/internal/provider"
)

func TestBudgetReplanKeepsTheLargestTripThatFits(t *testing.T) {
	req := demoRequest()
	req.Budget = 40000
	result, err := Build(context.Background(), catalog.Load(), nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FitsBudget || !result.Valid || !result.Adjusted || !result.Booked {
		t.Fatalf("fits %v valid %v adjusted %v booked %v issues %+v", result.FitsBudget, result.Valid, result.Adjusted, result.Booked, result.Issues)
	}
	if result.RequestedTotal != 50540 || result.Total != 35891 {
		t.Fatalf("requested %d total %d", result.RequestedTotal, result.Total)
	}
	if len(result.Stays) != 1 || result.Stays[0].City != "BOM" || result.Stays[0].Nights != 1 {
		t.Fatalf("stays %+v", result.Stays)
	}
	if !strings.Contains(result.Adjustment, "50540") || !strings.Contains(result.Adjustment, "last city") || !strings.Contains(result.FareNotice, "not confirmed tickets") {
		t.Fatalf("adjustment %s notice %s", result.Adjustment, result.FareNotice)
	}
	if result.Legs[0].Quote != "sample fare, not a confirmed ticket" || result.Legs[0].Provider != "Portage sample inventory" || result.Legs[0].RetrievedAt == "" {
		t.Fatalf("provenance %+v", result.Legs[0])
	}
	sawOver := false
	for _, cut := range result.Reductions {
		if !cut.Fits && cut.Total < result.RequestedTotal && cut.Total > req.Budget {
			sawOver = true
		}
	}
	if !sawOver {
		t.Fatalf("reductions %+v", result.Reductions)
	}
}

func TestNothingFitsExplainsTheMinimum(t *testing.T) {
	req := demoRequest()
	req.Budget = 10000
	result, err := Build(context.Background(), catalog.Load(), nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.FitsBudget || result.MinimumBudget != result.Total || result.RequestedTotal != 50540 {
		t.Fatalf("fits %v min %d total %d requested %d", result.FitsBudget, result.MinimumBudget, result.Total, result.RequestedTotal)
	}
	if result.Total >= 50540 || !strings.Contains(result.Adjustment, "No version") {
		t.Fatalf("%d %s", result.Total, result.Adjustment)
	}
	if sumLines(result) != result.Total {
		t.Fatalf("lines %d total %d", sumLines(result), result.Total)
	}
}

func TestDelhiGoaConnectsInsteadOfFailing(t *testing.T) {
	result, err := Build(context.Background(), catalog.Load(), nil, Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 2, Budget: 80000,
		Stops: []Stop{{City: "GOI", Nights: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FitsBudget || !result.Valid || result.Recovery == nil {
		t.Fatalf("fits %v valid %v recovery %+v issues %+v", result.FitsBudget, result.Valid, result.Recovery, result.Issues)
	}
	if len(result.Legs) < 3 || result.Legs[0].ID != "6E102" || result.Legs[1].ID != "6E330" {
		t.Fatalf("legs %+v", result.Legs)
	}
	if !strings.Contains(result.Adjustment, "Mumbai") || !strings.Contains(result.Recovery.DateNote, "another departure date") {
		t.Fatalf("adjustment %s note %s", result.Adjustment, result.Recovery.DateNote)
	}
	if result.Stays[0].City != "GOI" || result.Stays[0].Quote == "" {
		t.Fatalf("stay %+v", result.Stays[0])
	}
}

func TestDelhiGoaOverBudgetExplainsTheFloor(t *testing.T) {
	result, err := Build(context.Background(), catalog.Load(), nil, Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 2, Budget: 40000,
		Stops: []Stop{{City: "GOI", Nights: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FitsBudget || !strings.Contains(result.Adjustment, "No version") {
		t.Fatalf("fits %v %s", result.FitsBudget, result.Adjustment)
	}
	if result.MinimumBudget != result.Total || result.RequestedTotal < result.MinimumBudget {
		t.Fatalf("min %d total %d requested %d", result.MinimumBudget, result.Total, result.RequestedTotal)
	}
	if result.Recovery == nil || !strings.Contains(result.Recovery.Reason, "Delhi") {
		t.Fatalf("recovery %+v", result.Recovery)
	}
}

func TestMissingRouteExplainsDates(t *testing.T) {
	inventory := catalog.Inventory{
		Hotels: []catalog.Hotel{{ID: "h", City: "JAI", Name: "Inn", Nightly: 1000}},
	}
	result, err := Build(context.Background(), inventory, nil, Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 1, Budget: 50000,
		Stops: []Stop{{City: "JAI", Nights: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Booked || result.Recovery == nil || !strings.Contains(result.Recovery.DateNote, "another departure date") {
		t.Fatalf("%+v", result.Recovery)
	}
}

func TestLiveEmptyAndAdjacentDate(t *testing.T) {
	empty := &fakeFlights{}
	result, err := Build(context.Background(), catalog.Load(), empty, demoRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(joinReasons(result), "no bookable INR fares") {
		t.Fatalf("reasons %s", joinReasons(result))
	}
	recovered, err := Build(context.Background(), catalog.Inventory{
		Hotels: []catalog.Hotel{{ID: "h", City: "JAI", Name: "Inn", Nightly: 1000}},
	}, dateFlights{}, Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 1, Budget: 50000,
		Stops: []Stop{{City: "JAI", Nights: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	if recovered.Recovery != nil {
		for _, suggestion := range recovered.Recovery.Suggestions {
			if strings.Contains(suggestion.Label, "2026-12-11") && suggestion.Total == 1500 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("suggestions %+v", recovered.Recovery)
	}
}

type dateFlights struct{}

func (dateFlights) Enabled() bool { return true }

func (dateFlights) Search(_ context.Context, query provider.Query) ([]provider.Offer, error) {
	if query.Date != "2026-12-11" {
		return nil, nil
	}
	return []provider.Offer{{
		ID: "6E1", Carrier: "IndiGo", Origin: query.Origin, Destination: query.Destination,
		Total: 1500, Source: "live",
	}}, nil
}

func TestRequestBoundaries(t *testing.T) {
	base := demoRequest()
	cases := []Request{clone(base, func(r *Request) { r.Travelers = 0 }), clone(base, func(r *Request) { r.Travelers = 5 }), clone(base, func(r *Request) { r.Budget = 0 }), clone(base, func(r *Request) { r.Stops[0].Nights = 0 }), clone(base, func(r *Request) { r.Stops[0].Nights = 5 }), clone(base, func(r *Request) { r.Stops[0].Nights = 4; r.Stops[1].Nights = 4 }), clone(base, func(r *Request) { r.StartDate = "12-10-2026" })}
	for _, req := range cases {
		if _, err := Build(context.Background(), catalog.Load(), nil, req); err == nil {
			t.Fatalf("expected an error for %+v", req)
		}
	}
}

func TestTaxAndContingencyBoundaries(t *testing.T) {
	if hotelTaxRate(7500) != 12 || hotelTaxRate(7501) != 18 {
		t.Fatalf("rates %d %d", hotelTaxRate(7500), hotelTaxRate(7501))
	}
	if roundHalfUp(7500, 12) != 900 {
		t.Fatal(roundHalfUp(7500, 12))
	}
	if roundHalfUp(10, 12) != 1 || roundHalfUp(6, 8) != 0 || roundHalfUp(7, 8) != 1 || roundHalfUp(0, 8) != 0 || roundHalfUp(100, 0) != 0 {
		t.Fatalf("10→%d 6→%d 7→%d", roundHalfUp(10, 12), roundHalfUp(6, 8), roundHalfUp(7, 8))
	}
	inventory := catalog.Inventory{
		Transports: []catalog.Transport{
			{ID: "OUT", Mode: "flight", Origin: "DEL", Destination: "JAI", Carrier: "IndiGo", Depart: "08:00", Arrive: "09:10", Price: 100, FromStation: "DEL", ToStation: "JAI"},
			{ID: "BACK", Mode: "flight", Origin: "JAI", Destination: "DEL", Carrier: "IndiGo", Depart: "18:00", Arrive: "19:10", Price: 100, FromStation: "JAI", ToStation: "DEL"},
		},
		Hotels: []catalog.Hotel{{ID: "dear", City: "JAI", Name: "Dear", Nightly: 7501}},
	}
	result, err := Build(context.Background(), inventory, nil, Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 1, Budget: 20000,
		Stops: []Stop{{City: "JAI", Nights: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stays[0].TaxRate != 18 || result.Stays[0].Tax != roundHalfUp(7501, 18) {
		t.Fatalf("tax %+v", result.Stays[0])
	}
}

func clone(req Request, edit func(*Request)) Request {
	req.Stops = append([]Stop(nil), req.Stops...)
	edit(&req)
	return req
}
