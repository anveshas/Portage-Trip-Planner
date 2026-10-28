package provider

import (
	"testing"
	"time"
)

func TestNormalizeKeepsInrTotalAndDropsTheRest(t *testing.T) {
	body := []byte(`{
		"data": [
			{
				"id": "1",
				"price": {"currency": "INR", "grandTotal": "8400.40", "fees": [{"amount": "200.00", "type": "SUPPLIER"}]},
				"itineraries": [{"segments": [{
					"departure": {"iataCode": "DEL", "at": "2026-12-10T08:00:00"},
					"arrival": {"iataCode": "JAI", "at": "2026-12-10T09:10:00"},
					"carrierCode": "6E", "number": "201"
				}]}]
			},
			{
				"id": "2",
				"price": {"currency": "USD", "grandTotal": "100.00"},
				"itineraries": [{"segments": [{
					"departure": {"iataCode": "DEL", "at": "2026-12-10T08:00:00"},
					"arrival": {"iataCode": "JAI", "at": "2026-12-10T09:10:00"},
					"carrierCode": "XX", "number": "1"
				}]}]
			},
			{
				"id": "3",
				"price": {"currency": "INR", "grandTotal": ""},
				"itineraries": [{"segments": [{
					"departure": {"iataCode": "DEL", "at": "2026-12-10T10:00:00"},
					"arrival": {"iataCode": "JAI", "at": "2026-12-10T11:00:00"},
					"carrierCode": "6E", "number": "1"
				}]}]
			}
		]
	}`)
	offers, skips, err := Normalize(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(skips) != 2 {
		t.Fatalf("skips %+v", skips)
	}
	if len(offers) != 1 {
		t.Fatalf("offers %+v", offers)
	}
	offer := offers[0]
	if offer.ID != "6E201" || offer.Total != 8401 || offer.Fees != 200 || offer.Source != "live" {
		t.Fatalf("%+v", offer)
	}
	if offer.Total != 8401 {
		t.Fatal("fees must not be added on top of grandTotal")
	}
	if offer.Depart.Location() != ist || offer.Depart.Hour() != 8 {
		t.Fatalf("depart %s", offer.Depart)
	}
	if offer.Arrive.Sub(offer.Depart) != 70*time.Minute {
		t.Fatalf("duration %s", offer.Arrive.Sub(offer.Depart))
	}
}

func TestCurrencyConversionBoundaries(t *testing.T) {
	inr, err := ToINR("8400.40", "INR", nil)
	if err != nil || inr != 8401 {
		t.Fatalf("%d %v", inr, err)
	}
	whole, err := ToINR("8400.00", "inr", nil)
	if err != nil || whole != 8400 {
		t.Fatalf("%d %v", whole, err)
	}
	if _, err := ToINR("0", "INR", nil); err == nil {
		t.Fatal("zero is a missing price")
	}
	if _, err := ToINR("-5", "INR", nil); err == nil {
		t.Fatal("negative price")
	}
	if _, err := ToINR("10.00", "USD", nil); err == nil {
		t.Fatal("usd without a rate must not be treated as rupees")
	}
	converted, err := ToINR("10.00", "USD", map[string]float64{"USD": 83})
	if err != nil || converted != 830 {
		t.Fatalf("%d %v", converted, err)
	}
	penny, err := ToINR("0.01", "USD", map[string]float64{"USD": 83})
	if err != nil || penny != 1 {
		t.Fatalf("penny %d %v", penny, err)
	}
	fraction, err := ToINR("10.009", "USD", map[string]float64{"USD": 83})
	if err != nil || fraction != 831 {
		t.Fatalf("extra digits %d %v", fraction, err)
	}
	body := []byte(`{"data":[{"id":"9","price":{"currency":"USD","grandTotal":"10.00"},"itineraries":[{"segments":[{"departure":{"iataCode":"DEL","at":"2026-12-10T08:00:00"},"arrival":{"iataCode":"JAI","at":"2026-12-10T09:10:00"},"carrierCode":"6E","number":"1"}]}]}]}`)
	offers, skips, err := Normalize(body, map[string]float64{"USD": 83})
	if err != nil || len(skips) != 0 || len(offers) != 1 || offers[0].Total != 830 {
		t.Fatalf("offers %+v skips %+v err %v", offers, skips, err)
	}
	empty, skips, err := Normalize([]byte(`{"data":[]}`), nil)
	if err != nil || len(empty) != 0 || len(skips) != 0 {
		t.Fatalf("empty %+v %+v %v", empty, skips, err)
	}
}

func TestCeilRupees(t *testing.T) {
	whole, err := ceilRupees("8400.00")
	if err != nil || whole != 8400 {
		t.Fatalf("%d %v", whole, err)
	}
	up, err := ceilRupees("0.40")
	if err != nil || up != 1 {
		t.Fatalf("%d %v", up, err)
	}
	if _, err := ceilRupees("0"); err == nil {
		t.Fatal("zero is a missing price")
	}
}
