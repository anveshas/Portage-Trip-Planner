package plan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"portage/gate/internal/catalog"
	"portage/gate/internal/provider"
)

func Build(ctx context.Context, inventory catalog.Inventory, flights provider.Flights, req Request) (Result, error) {
	if err := checkRequest(req); err != nil {
		return Result{}, err
	}
	exact, err := buildExact(ctx, inventory, flights, req)
	if err != nil {
		var missed routeMiss
		if errors.As(err, &missed) {
			return explainMiss(ctx, inventory, flights, req, missed), nil
		}
		return Result{}, err
	}
	exact.RequestedTotal = exact.Total
	if exact.FitsBudget {
		exact.MinimumBudget = exact.Total
		if exact.Adjustment == "" && exact.Recovery != nil {
			exact.Adjustment = exact.Recovery.Reason
			if len(exact.Recovery.Suggestions) > 0 {
				exact.Adjustment += " " + exact.Recovery.Suggestions[0].Label + ". " + exact.Recovery.DateNote
			}
		}
		return exact, nil
	}
	variants := cheaperShapes(req)
	var reductions []Reduction
	var best *Result
	var floor *Result
	for _, candidate := range variants {
		if sameStops(candidate.Stops, req.Stops) {
			continue
		}
		built, buildErr := buildExact(ctx, inventory, flights, candidate)
		if buildErr != nil {
			continue
		}
		built.Report.APIFailures += exact.Report.APIFailures
		label := shapeLabel(candidate.Stops)
		reductions = append(reductions, Reduction{
			Label: label, Stops: candidate.Stops, Total: built.Total,
			Fits: built.FitsBudget && built.Valid, Saves: exact.Total - built.Total,
		})
		if floor == nil || built.Total < floor.Total {
			copy := built
			floor = &copy
		}
		if built.FitsBudget && built.Valid && (best == nil || betterFit(built, *best, req)) {
			copy := built
			best = &copy
		}
	}
	if best != nil {
		best.RequestedTotal = exact.Total
		best.MinimumBudget = best.Total
		best.Reductions = reductions
		best.Adjusted = true
		best.Adjustment = fmt.Sprintf(
			"The trip you asked for is ₹%d, which is over the ₹%d budget. The best plan that fits keeps %s at ₹%d. Best means the most requested cities, then a plan that still includes the last city, then the most nights, then the lower cost.",
			exact.Total, req.Budget, shapeLabel(stopsOf(*best)), best.Total,
		)
		if exact.Recovery != nil {
			best.Adjustment += " " + exact.Recovery.Reason
			if best.Recovery == nil {
				best.Recovery = exact.Recovery
			}
		}
		best.Steps = append([]Step{{
			Agent: "budget", Action: "replanned", Detail: best.Adjustment,
		}}, best.Steps...)
		return *best, nil
	}
	shown := exact
	if floor != nil && floor.Total < exact.Total {
		shown = *floor
	}
	shown.RequestedTotal = exact.Total
	shown.MinimumBudget = shown.Total
	shown.Reductions = reductions
	shown.Adjusted = shown.Total != exact.Total || exact.Adjusted
	shown.Adjustment = fmt.Sprintf(
		"No version of this trip fits ₹%d. The cheapest version is ₹%d. The trip you asked for is ₹%d.",
		req.Budget, shown.MinimumBudget, exact.Total,
	)
	if exact.Recovery != nil && shown.Recovery == nil {
		shown.Recovery = exact.Recovery
		shown.Adjustment += " " + exact.Recovery.Reason
	}
	return shown, nil
}

func explainMiss(ctx context.Context, inventory catalog.Inventory, flights provider.Flights, req Request, missed routeMiss) Result {
	retrieved := time.Now().In(ist)
	result := Result{
		OriginCode: req.Origin, Origin: cityName(req.Origin), Route: requestedRoute(req),
		Currency: "INR", Budget: req.Budget, Travelers: req.Travelers,
		Booked: false, FitsBudget: false, Valid: false, Adjusted: true,
		Legs: []Leg{}, Stays: []Stay{}, Lines: []Line{}, Alternatives: []Alternative{},
		Reductions: []Reduction{}, Issues: []Issue{}, Steps: []Step{},
		TrainNote:  "Trains are not booked. Check IRCTC for live availability.",
		Adjustment: missed.Error() + " " + sampleDateNote(missed.From, missed.To),
		Recovery: &Recovery{
			Reason:      missed.Error() + ".",
			DateNote:    sampleDateNote(missed.From, missed.To),
			Suggestions: dateSuggestions(ctx, flights, missed, req.Travelers),
		},
		FareNotice:  "No ride was booked, so nothing here is a confirmed fare.",
		RetrievedAt: retrieved.Format(time.RFC3339),
	}
	result.Steps = append(result.Steps, Step{
		Agent: "transport", Action: "found no direct ride", Detail: result.Adjustment,
	})
	return result
}

func dateSuggestions(ctx context.Context, flights provider.Flights, missed routeMiss, travelers int) []Suggestion {
	note := Suggestion{
		Label:  "Another date",
		Detail: sampleDateNote(missed.From, missed.To),
	}
	if flights == nil || !flights.Enabled() {
		return []Suggestion{note}
	}
	start, err := time.Parse("2006-01-02", missed.Date)
	if err != nil {
		return []Suggestion{note}
	}
	suggestions := []Suggestion{note}
	for _, shift := range []int{-1, 1} {
		day := start.AddDate(0, 0, shift).Format("2006-01-02")
		offers, err := flights.Search(ctx, provider.Query{Origin: missed.From, Destination: missed.To, Date: day, Travelers: travelers})
		if err != nil || len(offers) == 0 {
			continue
		}
		cheapest := offers[0]
		for _, offer := range offers[1:] {
			if offer.Total < cheapest.Total {
				cheapest = offer
			}
		}
		suggestions = append(suggestions, Suggestion{
			Label:  "Live fare on " + day,
			Detail: cheapest.Carrier + " " + cheapest.ID + " from Amadeus test. This is a search result, not a confirmed booking, and the rest of the trip was not repriced on that date.",
			Total:  cheapest.Total,
		})
	}
	return suggestions
}

func cheaperShapes(req Request) []Request {
	var shapes []Request
	if len(req.Stops) == 1 {
		for nights := req.Stops[0].Nights; nights >= 1; nights-- {
			shapes = append(shapes, withStops(req, []Stop{{City: req.Stops[0].City, Nights: nights}}))
		}
		return shapes
	}
	for first := req.Stops[0].Nights; first >= 1; first-- {
		for second := req.Stops[1].Nights; second >= 1; second-- {
			shapes = append(shapes, withStops(req, []Stop{
				{City: req.Stops[0].City, Nights: first},
				{City: req.Stops[1].City, Nights: second},
			}))
		}
	}
	for nights := req.Stops[0].Nights; nights >= 1; nights-- {
		shapes = append(shapes, withStops(req, []Stop{{City: req.Stops[0].City, Nights: nights}}))
	}
	for nights := req.Stops[1].Nights; nights >= 1; nights-- {
		shapes = append(shapes, withStops(req, []Stop{{City: req.Stops[1].City, Nights: nights}}))
	}
	return shapes
}

func sameStops(left, right []Stop) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].City != right[i].City || left[i].Nights != right[i].Nights {
			return false
		}
	}
	return true
}

func withStops(req Request, stops []Stop) Request {
	next := req
	next.Stops = stops
	return next
}

func betterFit(next, current Result, req Request) bool {
	nextCities, nextNights := coverage(next, req)
	currentCities, currentNights := coverage(current, req)
	if nextCities != currentCities {
		return nextCities > currentCities
	}
	nextPrimary := keepsPrimary(next, req)
	currentPrimary := keepsPrimary(current, req)
	if nextPrimary != currentPrimary {
		return nextPrimary
	}
	if nextNights != currentNights {
		return nextNights > currentNights
	}
	return next.Total < current.Total
}

func keepsPrimary(result Result, req Request) bool {
	if len(req.Stops) == 0 {
		return false
	}
	primary := req.Stops[len(req.Stops)-1].City
	for _, stay := range result.Stays {
		if stay.City == primary {
			return true
		}
	}
	return false
}

func coverage(result Result, req Request) (cities, nights int) {
	wanted := map[string]bool{}
	for _, stop := range req.Stops {
		wanted[stop.City] = true
	}
	for _, stay := range result.Stays {
		if wanted[stay.City] {
			cities++
		}
		nights += stay.Nights
	}
	return cities, nights
}

func stopsOf(result Result) []Stop {
	stops := make([]Stop, 0, len(result.Stays))
	for _, stay := range result.Stays {
		stops = append(stops, Stop{City: stay.City, Nights: stay.Nights})
	}
	return stops
}

func shapeLabel(stops []Stop) string {
	if len(stops) == 0 {
		return "no stay"
	}
	label := cityName(stops[0].City) + " for " + nightsWord(stops[0].Nights)
	for _, stop := range stops[1:] {
		label += " and " + cityName(stop.City) + " for " + nightsWord(stop.Nights)
	}
	return label
}

func nightsWord(nights int) string {
	if nights == 1 {
		return "1 night"
	}
	return fmt.Sprintf("%d nights", nights)
}

func requestedRoute(req Request) string {
	codes := []string{req.Origin}
	for _, stop := range req.Stops {
		codes = append(codes, stop.City)
	}
	codes = append(codes, req.Origin)
	return routeLabel(codes)
}
