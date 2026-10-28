// Package plan builds a budget-constrained India itinerary.
// Money, hotel nights, and transfer times are calculated here.
// Prose is allowed to explain the result and is not allowed to do the arithmetic.
package plan

import (
	"context"
	"fmt"
	"sort"
	"time"

	"portage/gate/internal/catalog"
	"portage/gate/internal/provider"
)

const (
	mealPerTravelerNight = 700
	localPerTravelerCity = 500
	contingencyPercent   = 8
)

var ist = time.FixedZone("IST", 5*3600+30*60)

var cityNames = map[string]string{
	"DEL": "Delhi",
	"JAI": "Jaipur",
	"BOM": "Mumbai",
	"GOI": "Goa",
}

type Stop struct {
	City   string `json:"city"`
	Nights int    `json:"nights"`
}

type Request struct {
	Origin    string `json:"origin"`
	StartDate string `json:"start_date"`
	Travelers int    `json:"travelers"`
	Budget    int    `json:"budget"`
	Stops     []Stop `json:"stops"`
}

type Step struct {
	Agent  string `json:"agent"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

type Line struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Amount      int    `json:"amount"`
	Source      string `json:"source"`
	Counted     bool   `json:"counted"`
	Provider    string `json:"provider"`
	RetrievedAt string `json:"retrieved_at"`
	ExpiresAt   string `json:"expires_at"`
	Currency    string `json:"currency"`
	DataStatus  string `json:"data_status"`
	Quote       string `json:"quote"`
}

type Leg struct {
	From        string `json:"from"`
	To          string `json:"to"`
	FromName    string `json:"from_name"`
	ToName      string `json:"to_name"`
	Date        string `json:"date"`
	Mode        string `json:"mode"`
	ID          string `json:"id"`
	Carrier     string `json:"carrier"`
	Depart      string `json:"depart"`
	Arrive      string `json:"arrive"`
	FromStation string `json:"from_station"`
	ToStation   string `json:"to_station"`
	PerPerson   int    `json:"per_person"`
	Total       int    `json:"total"`
	Source      string `json:"source"`
	Provider    string `json:"provider"`
	RetrievedAt string `json:"retrieved_at"`
	ExpiresAt   string `json:"expires_at"`
	Currency    string `json:"currency"`
	DataStatus  string `json:"data_status"`
	Quote       string `json:"quote"`
	Why         string `json:"why"`
}

type Stay struct {
	City        string `json:"city"`
	CityName    string `json:"city_name"`
	HotelID     string `json:"hotel_id"`
	Name        string `json:"name"`
	Area        string `json:"area"`
	CheckIn     string `json:"check_in"`
	CheckOut    string `json:"check_out"`
	Nights      int    `json:"nights"`
	Rooms       int    `json:"rooms"`
	Nightly     int    `json:"nightly"`
	RoomTotal   int    `json:"room_total"`
	TaxRate     int    `json:"tax_rate"`
	Tax         int    `json:"tax"`
	Source      string `json:"source"`
	Provider    string `json:"provider"`
	RetrievedAt string `json:"retrieved_at"`
	ExpiresAt   string `json:"expires_at"`
	Currency    string `json:"currency"`
	DataStatus  string `json:"data_status"`
	Quote       string `json:"quote"`
}

type Alternative struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Label     string `json:"label"`
	PerPerson int    `json:"per_person"`
	Source    string `json:"source"`
	Reason    string `json:"reason"`
}

type Issue struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type Rules struct {
	MealPerTravelerNight int      `json:"meal_per_traveler_night"`
	LocalPerTravelerCity int      `json:"local_per_traveler_city"`
	ContingencyPercent   int      `json:"contingency_percent"`
	HotelTax             string   `json:"hotel_tax"`
	BudgetScope          string   `json:"budget_scope"`
	HotelBasis           string   `json:"hotel_basis"`
	CostOrder            string   `json:"cost_order"`
	SameStationMinutes   int      `json:"same_station_minutes"`
	ChangeStationMinutes int      `json:"change_station_minutes"`
	Timing               string   `json:"timing"`
	Ranking              []string `json:"ranking"`
	PrimaryDestination   string   `json:"primary_destination"`
}

type Report struct {
	ValidationPassed bool `json:"validation_passed"`
	FitsBudget       bool `json:"fits_budget"`
	LineCount        int  `json:"line_count"`
	Live             int  `json:"live"`
	Sample           int  `json:"sample"`
	Estimated        int  `json:"estimated"`
	Calculated       int  `json:"calculated"`
	Manual           int  `json:"manual"`
	APIFailures      int  `json:"api_failures"`
}

type Reduction struct {
	Label string `json:"label"`
	Stops []Stop `json:"stops"`
	Total int    `json:"total"`
	Fits  bool   `json:"fits"`
	Saves int    `json:"saves"`
}

type Suggestion struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Total  int    `json:"total,omitempty"`
	Fits   bool   `json:"fits"`
	Stops  []Stop `json:"stops,omitempty"`
}

type Recovery struct {
	Reason      string       `json:"reason"`
	DateNote    string       `json:"date_note"`
	Suggestions []Suggestion `json:"suggestions"`
}

type Result struct {
	OriginCode     string        `json:"origin_code"`
	Origin         string        `json:"origin"`
	Route          string        `json:"route"`
	Currency       string        `json:"currency"`
	Budget         int           `json:"budget"`
	Total          int           `json:"total"`
	RequestedTotal int           `json:"requested_total"`
	MinimumBudget  int           `json:"minimum_budget"`
	FitsBudget     bool          `json:"fits_budget"`
	Valid          bool          `json:"valid"`
	Booked         bool          `json:"booked"`
	Adjusted       bool          `json:"adjusted"`
	Adjustment     string        `json:"adjustment"`
	FareNotice     string        `json:"fare_notice"`
	RetrievedAt    string        `json:"retrieved_at"`
	Travelers      int           `json:"travelers"`
	Legs           []Leg         `json:"legs"`
	Stays          []Stay        `json:"stays"`
	Lines          []Line        `json:"lines"`
	Alternatives   []Alternative `json:"alternatives"`
	Reductions     []Reduction   `json:"reductions"`
	Recovery       *Recovery     `json:"recovery,omitempty"`
	Issues         []Issue       `json:"issues"`
	Steps          []Step        `json:"steps"`
	TrainNote      string        `json:"train_note"`
	Rules          Rules         `json:"rules"`
	Report         Report        `json:"report"`
}

type option struct {
	mode        string
	id          string
	carrier     string
	depart      time.Time
	arrive      time.Time
	fromStation string
	toStation   string
	perPerson   int
	total       int
	source      string
	overnight   bool
}

func buildExact(ctx context.Context, inventory catalog.Inventory, flights provider.Flights, req Request) (Result, error) {
	if err := checkRequest(req); err != nil {
		return Result{}, err
	}
	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return Result{}, fmt.Errorf("start_date must be YYYY-MM-DD")
	}
	route := []string{req.Origin}
	for _, stop := range req.Stops {
		route = append(route, stop.City)
	}
	route = append(route, req.Origin)

	result := Result{
		OriginCode:   req.Origin,
		Origin:       cityName(req.Origin),
		Route:        routeLabel(route),
		Currency:     "INR",
		Budget:       req.Budget,
		Travelers:    req.Travelers,
		Legs:         []Leg{},
		Stays:        []Stay{},
		Lines:        []Line{},
		Alternatives: []Alternative{},
		Reductions:   []Reduction{},
		Issues:       []Issue{},
		Steps:        []Step{},
		TrainNote:    "Trains are not booked. Check IRCTC for live availability. Sample estimates are listed with the other options and are not part of the total.",
		Rules: Rules{
			MealPerTravelerNight: mealPerTravelerNight,
			LocalPerTravelerCity: localPerTravelerCity,
			ContingencyPercent:   contingencyPercent,
			HotelTax:             "12% of the room total when the nightly rate is ₹7,500 or less, otherwise 18%",
			BudgetScope:          "one total for the whole group, not an amount per person",
			HotelBasis:           "the nightly rate is per room, and one room holds two travelers, rounded up",
			CostOrder:            "transport, then room total, then hotel tax on the room total only, then meals, then local transport, then 8% contingency on that sum once",
			SameStationMinutes:   90,
			ChangeStationMinutes: 120,
			Timing:               "connections use departure and arrival instants, including time zones; an overnight arrival is not booked",
			Ranking:              []string{"most requested cities", "keep the last requested city", "most nights", "lower cost"},
			PrimaryDestination:   "the last city in the request",
		},
	}

	cursor := start
	previous := req.Origin
	var arrived time.Time
	arrivedStation := ""
	rooms := (req.Travelers + 1) / 2
	for i, stop := range req.Stops {
		date := cursor.Format("2006-01-02")
		hop, alts, failures, via, err := travel(ctx, inventory, flights, previous, stop.City, date, req.Travelers, arrived, arrivedStation, i == 0)
		result.Report.APIFailures += failures
		if failures > 0 {
			result.Steps = append(result.Steps, Step{
				Agent: "transport", Action: "kept the sample fares",
				Detail: "the live flight search failed for " + cityName(previous) + " to " + cityName(stop.City),
			})
		}
		result.Alternatives = append(result.Alternatives, alts...)
		if err != nil {
			return Result{}, err
		}
		if via != "" {
			result.Adjusted = true
			result.Recovery = &Recovery{
				Reason:   fmt.Sprintf("No direct same-day flight or bus from %s to %s.", cityName(previous), cityName(stop.City)),
				DateNote: sampleDateNote(previous, stop.City),
				Suggestions: []Suggestion{{
					Label:  "Connect through " + cityName(via) + " on the same day",
					Detail: hopDetail(hop),
					Stops:  req.Stops,
				}},
			}
		}
		for _, leg := range hop {
			result.Legs = append(result.Legs, leg)
			result.Lines = append(result.Lines, transportLine(leg))
			result.Steps = append(result.Steps, Step{
				Agent: "transport", Action: "chose " + leg.Carrier + " " + leg.ID,
				Detail: leg.Why + fmt.Sprintf(" Group total ₹%d.", leg.Total),
			})
		}
		leg := hop[len(hop)-1]
		arrived, _ = time.Parse(time.RFC3339, leg.Arrive)

		hotel, err := cheapestHotel(inventory.HotelsIn(stop.City))
		if err != nil {
			return Result{}, fmt.Errorf("no hotel in %s", cityName(stop.City))
		}
		checkIn := cursor
		checkOut := cursor.AddDate(0, 0, stop.Nights)
		roomTotal := hotel.Nightly * stop.Nights * rooms
		rate := hotelTaxRate(hotel.Nightly)
		tax := roundHalfUp(roomTotal, rate)
		stay := Stay{
			City: stop.City, CityName: cityName(stop.City), HotelID: hotel.ID, Name: hotel.Name, Area: hotel.Area,
			CheckIn: checkIn.Format("2006-01-02"), CheckOut: checkOut.Format("2006-01-02"),
			Nights: stop.Nights, Rooms: rooms, Nightly: hotel.Nightly, RoomTotal: roomTotal,
			TaxRate: rate, Tax: tax, Source: "sample",
		}
		result.Stays = append(result.Stays, stay)
		result.Lines = append(result.Lines,
			Line{Kind: "hotel", Label: hotel.Name + " for " + stay.CheckIn, Amount: roomTotal, Source: "sample", Counted: true},
			Line{Kind: "tax", Label: fmt.Sprintf("Hotel tax, %d%% of %s", rate, hotel.Name), Amount: tax, Source: "calculated", Counted: true},
		)
		result.Steps = append(result.Steps, Step{
			Agent: "stay", Action: "chose " + hotel.Name,
			Detail: fmt.Sprintf("cheapest room in %s, ₹%d a night, %d room, %d%% tax ₹%d. Dearer rooms were left out.", cityName(stop.City), hotel.Nightly, rooms, rate, tax),
		})
		for _, other := range inventory.HotelsIn(stop.City) {
			if other.ID == hotel.ID || other.Nightly <= 0 {
				continue
			}
			result.Alternatives = append(result.Alternatives, Alternative{
				From: stop.City, To: stop.City, Label: other.Name, PerPerson: other.Nightly, Source: "sample",
				Reason: "dearer than the room that was booked",
			})
		}
		cursor = checkOut
		previous = stop.City
		arrivedStation = leg.ToStation
	}

	homeDate := cursor.Format("2006-01-02")
	hop, alts, failures, via, err := travel(ctx, inventory, flights, previous, req.Origin, homeDate, req.Travelers, arrived, arrivedStation, false)
	result.Report.APIFailures += failures
	if failures > 0 {
		result.Steps = append(result.Steps, Step{
			Agent: "transport", Action: "kept the sample fares",
			Detail: "the live flight search failed for " + cityName(previous) + " to " + cityName(req.Origin),
		})
	}
	result.Alternatives = append(result.Alternatives, alts...)
	if err != nil {
		return Result{}, err
	}
	if via != "" {
		result.Adjusted = true
		if result.Recovery == nil {
			result.Recovery = &Recovery{Suggestions: []Suggestion{}}
		}
		result.Recovery.Reason = stringsJoin(result.Recovery.Reason, fmt.Sprintf("No direct same-day flight or bus from %s to %s.", cityName(previous), cityName(req.Origin)))
		result.Recovery.DateNote = sampleDateNote(previous, req.Origin)
		result.Recovery.Suggestions = append(result.Recovery.Suggestions, Suggestion{
			Label: "Connect through " + cityName(via) + " on the way home", Detail: hopDetail(hop),
		})
	}
	for _, leg := range hop {
		result.Legs = append(result.Legs, leg)
		result.Lines = append(result.Lines, transportLine(leg))
		result.Steps = append(result.Steps, Step{
			Agent: "transport", Action: "chose " + leg.Carrier + " " + leg.ID,
			Detail: leg.Why + fmt.Sprintf(" Group total ₹%d.", leg.Total),
		})
	}

	nights := 0
	for _, stop := range req.Stops {
		nights += stop.Nights
	}
	meals := mealPerTravelerNight * req.Travelers * nights
	local := localPerTravelerCity * req.Travelers * len(req.Stops)
	result.Lines = append(result.Lines,
		Line{Kind: "meals", Label: fmt.Sprintf("Meals, ₹%d per traveler for each night away", mealPerTravelerNight), Amount: meals, Source: "estimated", Counted: true},
		Line{Kind: "local", Label: fmt.Sprintf("Local transport, ₹%d per traveler in each city", localPerTravelerCity), Amount: local, Source: "estimated", Counted: true},
	)
	subtotal := 0
	for _, line := range result.Lines {
		if line.Counted {
			subtotal += line.Amount
		}
	}
	contingency := roundHalfUp(subtotal, contingencyPercent)
	result.Lines = append(result.Lines, Line{
		Kind: "contingency", Label: fmt.Sprintf("Contingency, %d%% of the other booked costs", contingencyPercent),
		Amount: contingency, Source: "calculated", Counted: true,
	})
	result.Total = subtotal + contingency
	result.FitsBudget = result.Total <= req.Budget
	fitWord := "inside"
	if !result.FitsBudget {
		fitWord = "over"
	}
	result.Steps = append(result.Steps, Step{
		Agent: "budget", Action: "added the booked lines",
		Detail: fmt.Sprintf("₹%d is %s the ₹%d budget. Meals, local transport, hotel tax, and an 8%% contingency are included. Nothing with a missing price was treated as free.", result.Total, fitWord, req.Budget),
	})
	result.Issues = Validate(result)
	result.Valid = len(result.Issues) == 0
	result.Steps = append(result.Steps, Step{
		Agent: "validate", Action: statusWord(result.Valid),
		Detail: validationDetail(result),
	})
	result.Report.ValidationPassed = result.Valid
	result.Report.FitsBudget = result.FitsBudget
	result.Report.LineCount = len(result.Lines)
	for _, line := range result.Lines {
		switch line.Source {
		case "live":
			result.Report.Live++
		case "sample":
			result.Report.Sample++
		case "estimated":
			result.Report.Estimated++
		case "calculated":
			result.Report.Calculated++
		}
	}
	for _, alt := range result.Alternatives {
		if alt.Source == "manual" {
			result.Report.Manual++
		}
	}
	finish(&result, time.Now().In(ist))
	return result, nil
}

func statusWord(valid bool) string {
	if valid {
		return "passed"
	}
	return "failed"
}

func validationDetail(result Result) string {
	if result.Valid {
		return "transfer buffers, hotel nights, date order, and the line-item total all match"
	}
	return result.Issues[0].Detail
}

func checkRequest(req Request) error {
	if req.Travelers < 1 || req.Travelers > 4 {
		return fmt.Errorf("travelers must be between 1 and 4")
	}
	if req.Budget <= 0 {
		return fmt.Errorf("budget must be greater than zero")
	}
	if len(req.Stops) < 1 || len(req.Stops) > 2 {
		return fmt.Errorf("plan one or two cities")
	}
	if _, ok := cityNames[req.Origin]; !ok {
		return fmt.Errorf("unknown city %s", req.Origin)
	}
	nights := 0
	previous := req.Origin
	for _, stop := range req.Stops {
		if _, ok := cityNames[stop.City]; !ok {
			return fmt.Errorf("unknown city %s", stop.City)
		}
		if stop.City == previous {
			return fmt.Errorf("each city has to be different from the one before it")
		}
		if stop.Nights < 1 || stop.Nights > 4 {
			return fmt.Errorf("each stay must be 1 to 4 nights")
		}
		nights += stop.Nights
		previous = stop.City
	}
	if nights > 7 {
		return fmt.Errorf("trips must be 7 nights or shorter")
	}
	return nil
}

func chooseLeg(ctx context.Context, inventory catalog.Inventory, flights provider.Flights, from, to, date string, travelers int, previous time.Time, previousStation string, first bool) (Leg, []Alternative, int, error) {
	var options []option
	var skipped []Alternative
	failures := 0
	for _, item := range inventory.Between(from, to) {
		opt, alt, ok := sampleOption(item, date, travelers)
		if !ok {
			skipped = append(skipped, alt)
			continue
		}
		options = append(options, opt)
	}
	if flights != nil && flights.Enabled() {
		offers, err := flights.Search(ctx, provider.Query{Origin: from, Destination: to, Date: date, Travelers: travelers})
		if err != nil {
			failures++
		} else if len(offers) == 0 {
			skipped = append(skipped, Alternative{
				From: from, To: to, Label: "Amadeus test", Source: "live",
				Reason: "the live search returned no bookable INR fares",
			})
		} else {
			for _, offer := range offers {
				opt, alt, ok := liveOption(offer, from, to, date, travelers)
				if !ok {
					skipped = append(skipped, alt)
					continue
				}
				options = append(options, opt)
			}
		}
	}
	if train, ok := inventory.Train(from, to); ok {
		skipped = append(skipped, Alternative{
			From: from, To: to, Label: "IRCTC " + cityName(from) + " to " + cityName(to),
			PerPerson: train.PerPerson, Source: "manual",
			Reason: "check IRCTC; this estimate is not a live fare and is not booked",
		})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].total == options[j].total {
			return options[i].id < options[j].id
		}
		return options[i].total < options[j].total
	})
	chosen, rejected := selectOption(options, previous, previousStation, first)
	skipped = append(skipped, rejected...)
	if chosen == nil {
		return Leg{}, skipped, failures, routeMiss{From: from, To: to, Date: date}
	}
	blocked := map[string]bool{}
	for _, alt := range rejected {
		blocked[alt.Label+"|"+alt.Source] = true
	}
	for _, opt := range options {
		label := opt.carrier + " " + opt.id
		if opt.id == chosen.id && opt.source == chosen.source {
			continue
		}
		if blocked[label+"|"+opt.source] {
			continue
		}
		skipped = append(skipped, Alternative{
			From: from, To: to, Label: label, PerPerson: opt.perPerson, Source: opt.source,
			Reason: "not the cheapest option that can be booked",
		})
	}
	leg := Leg{
		From: from, To: to, FromName: cityName(from), ToName: cityName(to), Date: date,
		Mode: chosen.mode, ID: chosen.id, Carrier: chosen.carrier,
		Depart: chosen.depart.Format(time.RFC3339), Arrive: chosen.arrive.Format(time.RFC3339),
		FromStation: chosen.fromStation, ToStation: chosen.toStation,
		PerPerson: chosen.perPerson, Total: chosen.total, Source: chosen.source,
		Why: fmt.Sprintf("cheapest same-day %s that clears the transfer buffer, %s fare", chosen.mode, chosen.source),
	}
	return leg, skipped, failures, nil
}

func selectOption(options []option, previous time.Time, previousStation string, first bool) (*option, []Alternative) {
	var rejected []Alternative
	for i := range options {
		opt := &options[i]
		if !first && !previous.IsZero() {
			need := changeStationBuffer
			if previousStation != "" && previousStation == opt.fromStation {
				need = sameStationBuffer
			}
			if opt.depart.Sub(previous) < need {
				rejected = append(rejected, Alternative{
					From: "", To: "", Label: opt.carrier + " " + opt.id, PerPerson: opt.perPerson, Source: opt.source,
					Reason: "departs before the transfer buffer after the previous arrival",
				})
				continue
			}
		}
		return opt, rejected
	}
	return nil, rejected
}

func sampleOption(item catalog.Transport, date string, travelers int) (option, Alternative, bool) {
	label := item.Carrier + " " + item.ID
	if item.Price <= 0 {
		return option{}, Alternative{From: item.Origin, To: item.Destination, Label: label, Source: "sample", Reason: "missing price, so it was not treated as free"}, false
	}
	depart := onDate(date, item.Depart)
	arrive := onDate(date, item.Arrive)
	if item.Overnight || !arrive.After(depart) {
		arrive = arrive.AddDate(0, 0, 1)
		return option{}, Alternative{From: item.Origin, To: item.Destination, Label: label, PerPerson: item.Price, Source: "sample", Reason: "arrives the next day, which would add a night this stay does not include"}, false
	}
	return option{
		mode: item.Mode, id: item.ID, carrier: item.Carrier, depart: depart, arrive: arrive,
		fromStation: item.FromStation, toStation: item.ToStation,
		perPerson: item.Price, total: item.Price * travelers, source: "sample",
	}, Alternative{}, true
}

func liveOption(offer provider.Offer, from, to, date string, travelers int) (option, Alternative, bool) {
	label := offer.Carrier + " " + offer.ID
	if offer.Total <= 0 || offer.Source != "live" {
		return option{}, Alternative{From: from, To: to, Label: label, Source: "live", Reason: "missing price, so it was not treated as free"}, false
	}
	depart := offer.Depart.In(ist)
	arrive := offer.Arrive.In(ist)
	if depart.Format("2006-01-02") != date {
		return option{}, Alternative{From: from, To: to, Label: label, Source: "live", Reason: "departs on a different date"}, false
	}
	if !arrive.After(depart) || arrive.Format("2006-01-02") != depart.Format("2006-01-02") {
		return option{}, Alternative{From: from, To: to, Label: label, Source: "live", Reason: "arrives the next day, which would add a night this stay does not include"}, false
	}
	per := 0
	if travelers > 0 && offer.Total%travelers == 0 {
		per = offer.Total / travelers
	}
	return option{
		mode: "flight", id: offer.ID, carrier: offer.Carrier, depart: depart, arrive: arrive,
		fromStation: from, toStation: to, perPerson: per, total: offer.Total, source: "live",
	}, Alternative{}, true
}

func onDate(date, clock string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, ist)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func cheapestHotel(hotels []catalog.Hotel) (catalog.Hotel, error) {
	var priced []catalog.Hotel
	for _, hotel := range hotels {
		if hotel.Nightly > 0 {
			priced = append(priced, hotel)
		}
	}
	if len(priced) == 0 {
		return catalog.Hotel{}, fmt.Errorf("none")
	}
	sort.Slice(priced, func(i, j int) bool {
		if priced[i].Nightly == priced[j].Nightly {
			return priced[i].ID < priced[j].ID
		}
		return priced[i].Nightly < priced[j].Nightly
	})
	return priced[0], nil
}

func cityName(code string) string {
	if name, ok := cityNames[code]; ok {
		return name
	}
	return code
}

func routeLabel(codes []string) string {
	label := cityName(codes[0])
	for _, code := range codes[1:] {
		label += " → " + cityName(code)
	}
	return label
}
