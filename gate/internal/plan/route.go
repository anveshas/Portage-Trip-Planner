package plan

import (
	"context"
	"fmt"
	"sort"
	"time"

	"portage/gate/internal/catalog"
	"portage/gate/internal/provider"
)

type routeMiss struct {
	From string
	To   string
	Date string
}

func (e routeMiss) Error() string {
	return fmt.Sprintf("no same-day flight or bus from %s to %s", cityName(e.From), cityName(e.To))
}

func sampleDateNote(from, to string) string {
	return fmt.Sprintf("Sample fares are the same on every date, so another departure date does not add a direct %s to %s ride. A different date only helps when a live search returns one.", cityName(from), cityName(to))
}

func travel(ctx context.Context, inventory catalog.Inventory, flights provider.Flights, from, to, date string, travelers int, previous time.Time, previousStation string, first bool) ([]Leg, []Alternative, int, string, error) {
	leg, alts, failures, err := chooseLeg(ctx, inventory, flights, from, to, date, travelers, previous, previousStation, first)
	if err == nil {
		return []Leg{leg}, alts, failures, "", nil
	}
	missed, ok := err.(routeMiss)
	if !ok {
		return nil, alts, failures, "", err
	}
	hop, viaAlts, viaFailures, via, viaErr := connectVia(ctx, inventory, flights, from, to, date, travelers, previous, previousStation, first)
	alts = append(alts, viaAlts...)
	failures += viaFailures
	if viaErr != nil {
		return nil, alts, failures, "", missed
	}
	return hop, alts, failures, via, nil
}

func connectVia(ctx context.Context, inventory catalog.Inventory, flights provider.Flights, from, to, date string, travelers int, previous time.Time, previousStation string, first bool) ([]Leg, []Alternative, int, string, error) {
	var codes []string
	for code := range cityNames {
		if code != from && code != to {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	var best []Leg
	var bestAlts []Alternative
	var bestVia string
	bestCost := 0
	failures := 0
	for _, via := range codes {
		firstLeg, alts1, fail1, err1 := chooseLeg(ctx, inventory, flights, from, via, date, travelers, previous, previousStation, first)
		failures += fail1
		if err1 != nil {
			continue
		}
		arrived, err := time.Parse(time.RFC3339, firstLeg.Arrive)
		if err != nil {
			continue
		}
		secondLeg, alts2, fail2, err2 := chooseLeg(ctx, inventory, flights, via, to, date, travelers, arrived, firstLeg.ToStation, false)
		failures += fail2
		if err2 != nil {
			continue
		}
		cost := firstLeg.Total + secondLeg.Total
		if bestVia == "" || cost < bestCost || (cost == bestCost && via < bestVia) {
			best = []Leg{firstLeg, secondLeg}
			bestAlts = append(append([]Alternative{}, alts1...), alts2...)
			bestVia = via
			bestCost = cost
		}
	}
	if bestVia == "" {
		return nil, nil, failures, "", fmt.Errorf("no connection")
	}
	return best, bestAlts, failures, bestVia, nil
}

func hopDetail(hop []Leg) string {
	if len(hop) == 0 {
		return ""
	}
	detail := hop[0].Carrier + " " + hop[0].ID
	for _, leg := range hop[1:] {
		detail += ", then " + leg.Carrier + " " + leg.ID
	}
	return detail
}

func stringsJoin(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + " " + right
}
