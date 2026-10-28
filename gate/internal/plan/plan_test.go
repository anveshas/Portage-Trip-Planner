package plan

import (
	"context"
	"strings"
	"testing"
	"time"

	"portage/gate/internal/catalog"
	"portage/gate/internal/provider"
)

func demoRequest() Request {
	return Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 2, Budget: 80000,
		Stops: []Stop{{City: "JAI", Nights: 2}, {City: "BOM", Nights: 2}},
	}
}

func buildDemo(t *testing.T) Result {
	t.Helper()
	result, err := Build(context.Background(), catalog.Load(), nil, demoRequest())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDemoTripFitsAndBalances(t *testing.T) {
	result := buildDemo(t)
	if result.Route != "Delhi → Jaipur → Mumbai → Delhi" {
		t.Fatalf("route %s", result.Route)
	}
	if len(result.Legs) != 3 || result.Legs[0].ID != "RB101" || result.Legs[1].ID != "6E440" || result.Legs[2].ID != "6E880" {
		t.Fatalf("legs %+v", result.Legs)
	}
	if result.Legs[0].Total != 1700 || result.Legs[0].Source != "sample" {
		t.Fatalf("bus %+v", result.Legs[0])
	}
	if len(result.Stays) != 2 || result.Stays[0].HotelID != "jai-station" || result.Stays[1].HotelID != "bom-bandra" {
		t.Fatalf("stays %+v", result.Stays)
	}
	if result.Stays[0].Tax != 432 || result.Stays[1].Tax != 864 {
		t.Fatalf("tax %+v", result.Stays)
	}
	if result.Total != 50540 || !result.FitsBudget || !result.Valid {
		t.Fatalf("total %d fits %v valid %v issues %+v", result.Total, result.FitsBudget, result.Valid, result.Issues)
	}
	if sumLines(result) != result.Total {
		t.Fatalf("lines %d total %d", sumLines(result), result.Total)
	}
	if result.Report.Live != 0 || result.Report.Sample != 5 || result.Report.Estimated != 2 || result.Report.Calculated != 3 || result.Report.Manual != 3 {
		t.Fatalf("report %+v", result.Report)
	}
	reasons := joinReasons(result)
	for _, want := range []string{"missing price", "arrives the next day", "check IRCTC", "not the cheapest option"} {
		if !strings.Contains(reasons, want) {
			t.Fatalf("missing %q in %s", want, reasons)
		}
	}
}

func TestTightBudgetReplansToATripThatFits(t *testing.T) {
	req := demoRequest()
	req.Budget = 40000
	result, err := Build(context.Background(), catalog.Load(), nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FitsBudget || !result.Valid || result.Total != 35891 || result.RequestedTotal != 50540 {
		t.Fatalf("total %d requested %d fits %v valid %v", result.Total, result.RequestedTotal, result.FitsBudget, result.Valid)
	}
	if len(result.Stays) != 1 || result.Stays[0].HotelID != "bom-bandra" || result.Stays[0].Nights != 1 {
		t.Fatalf("stay %+v", result.Stays)
	}
}

func TestOneStopReturn(t *testing.T) {
	req := Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 2, Budget: 80000,
		Stops: []Stop{{City: "JAI", Nights: 2}},
	}
	result, err := Build(context.Background(), catalog.Load(), nil, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Legs[1].ID != "6E220" || result.Total != 18935 || !result.Valid {
		t.Fatalf("%+v total %d", result.Legs, result.Total)
	}
}

func TestTightConnectionIsNotBooked(t *testing.T) {
	inventory := catalog.Inventory{
		Transports: []catalog.Transport{
			{ID: "BUS", Mode: "bus", Origin: "DEL", Destination: "JAI", Carrier: "RedBus", Depart: "06:00", Arrive: "22:30", Price: 500, FromStation: "DEL-ISBT", ToStation: "JAI-SINDHI"},
			{ID: "EARLY", Mode: "flight", Origin: "JAI", Destination: "DEL", Carrier: "IndiGo", Depart: "00:10", Arrive: "01:20", Price: 1000, FromStation: "JAI", ToStation: "DEL"},
			{ID: "LATER", Mode: "flight", Origin: "JAI", Destination: "DEL", Carrier: "IndiGo", Depart: "08:00", Arrive: "09:10", Price: 5000, FromStation: "JAI", ToStation: "DEL"},
		},
		Hotels: []catalog.Hotel{{ID: "h", City: "JAI", Name: "Inn", Nightly: 1000, Area: "Station"}},
	}
	result, err := Build(context.Background(), inventory, nil, Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 1, Budget: 50000,
		Stops: []Stop{{City: "JAI", Nights: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Legs[1].ID != "LATER" || !result.Valid {
		t.Fatalf("leg %+v issues %+v", result.Legs[1], result.Issues)
	}
	if !strings.Contains(joinReasons(result), "transfer buffer") {
		t.Fatalf("reasons %s", joinReasons(result))
	}
}

func TestUnknownCity(t *testing.T) {
	req := demoRequest()
	req.Stops = []Stop{{City: "PAR", Nights: 2}}
	if _, err := Build(context.Background(), catalog.Load(), nil, req); err == nil {
		t.Fatal("expected an error")
	}
}

type fakeFlights struct {
	offers []provider.Offer
	err    error
	calls  int
}

func (f *fakeFlights) Enabled() bool { return true }

func (f *fakeFlights) Search(_ context.Context, query provider.Query) ([]provider.Offer, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if query.Origin == "DEL" && query.Destination == "JAI" {
		return f.offers, nil
	}
	return nil, nil
}

func TestLiveFareReplacesSampleWhenCheaper(t *testing.T) {
	depart := time.Date(2026, 12, 10, 8, 0, 0, 0, ist)
	arrive := time.Date(2026, 12, 10, 9, 10, 0, 0, ist)
	flights := &fakeFlights{offers: []provider.Offer{{
		ID: "6E777", Carrier: "IndiGo", Origin: "DEL", Destination: "JAI",
		Depart: depart, Arrive: arrive, Total: 1000, Source: "live",
	}}}
	result, err := Build(context.Background(), catalog.Load(), flights, demoRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Legs[0].ID != "6E777" || result.Legs[0].Source != "live" || result.Legs[0].Total != 1000 {
		t.Fatalf("leg %+v", result.Legs[0])
	}
	if result.Total != 49784 || result.Report.Live != 1 || result.Report.APIFailures != 0 || !result.Valid {
		t.Fatalf("total %d report %+v issues %+v", result.Total, result.Report, result.Issues)
	}
}

func TestLiveFailureKeepsSample(t *testing.T) {
	flights := &fakeFlights{err: context.DeadlineExceeded}
	result, err := Build(context.Background(), catalog.Load(), flights, demoRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Legs[0].ID != "RB101" || result.Total != 50540 || result.Report.APIFailures != 3 {
		t.Fatalf("leg %s total %d failures %d", result.Legs[0].ID, result.Total, result.Report.APIFailures)
	}
	if flights.calls != 3 {
		t.Fatalf("calls %d", flights.calls)
	}
}

func sumLines(result Result) int {
	total := 0
	for _, line := range result.Lines {
		if line.Counted {
			total += line.Amount
		}
	}
	return total
}

func joinReasons(result Result) string {
	parts := make([]string, 0, len(result.Alternatives))
	for _, alt := range result.Alternatives {
		parts = append(parts, alt.Label+" "+alt.Reason)
	}
	return strings.Join(parts, "\n")
}
