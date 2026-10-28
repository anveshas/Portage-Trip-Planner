package catalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed inventory.json
var raw []byte

// Transport is one bookable same-day flight or bus. Price is per traveler, in INR.
// A zero price is missing data and must not be treated as free.
type Transport struct {
	ID          string `json:"id"`
	Mode        string `json:"mode"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	Carrier     string `json:"carrier"`
	Depart      string `json:"depart"`
	Arrive      string `json:"arrive"`
	Price       int    `json:"price"`
	FromStation string `json:"from_station"`
	ToStation   string `json:"to_station"`
	Overnight   bool   `json:"overnight"`
}

// TrainEstimate is an unpublished planning number. It is not a live IRCTC fare.
type TrainEstimate struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	PerPerson   int    `json:"per_person"`
}

type Hotel struct {
	ID      string `json:"id"`
	City    string `json:"city"`
	Name    string `json:"name"`
	Nightly int    `json:"nightly"`
	Area    string `json:"area"`
}

type Inventory struct {
	Transports []Transport     `json:"transports"`
	Trains     []TrainEstimate `json:"trains"`
	Hotels     []Hotel         `json:"hotels"`
}

func Load() Inventory {
	var inventory Inventory
	if err := json.Unmarshal(raw, &inventory); err != nil {
		panic(err)
	}
	return inventory
}

func (inv Inventory) Between(origin, dest string) []Transport {
	var matched []Transport
	for _, item := range inv.Transports {
		if item.Origin == origin && item.Destination == dest {
			matched = append(matched, item)
		}
	}
	return matched
}

func (inv Inventory) Train(origin, dest string) (TrainEstimate, bool) {
	for _, item := range inv.Trains {
		if item.Origin == origin && item.Destination == dest {
			return item, true
		}
	}
	return TrainEstimate{}, false
}

func (inv Inventory) HotelsIn(city string) []Hotel {
	var matched []Hotel
	for _, hotel := range inv.Hotels {
		if hotel.City == city {
			matched = append(matched, hotel)
		}
	}
	return matched
}
