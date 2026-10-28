package plan

import (
	"fmt"
	"time"
)

const (
	sameStationBuffer   = 90 * time.Minute
	changeStationBuffer = 120 * time.Minute
)

func Validate(result Result) []Issue {
	var issues []Issue
	counted := 0
	for _, line := range result.Lines {
		if !line.Counted {
			continue
		}
		counted += line.Amount
		if line.Amount <= 0 {
			issues = append(issues, Issue{Code: "missing_price", Detail: line.Label + " is booked without a price"})
		}
		if line.Kind == "train" && line.Source != "manual" {
			issues = append(issues, Issue{Code: "train_marked_live", Detail: "a train fare is marked " + line.Source})
		}
	}
	if counted != result.Total {
		issues = append(issues, Issue{
			Code:   "total_mismatch",
			Detail: fmt.Sprintf("booked lines add to ₹%d and the total says ₹%d", counted, result.Total),
		})
	}
	if result.FitsBudget != (result.Total <= result.Budget) {
		issues = append(issues, Issue{Code: "budget_flag", Detail: "fits_budget does not match the total and the budget"})
	}
	for _, leg := range result.Legs {
		if leg.Mode == "train" && leg.Source != "manual" {
			issues = append(issues, Issue{Code: "train_marked_live", Detail: leg.ID + " is a train marked " + leg.Source})
		}
		if leg.DataStatus == "expired" || leg.DataStatus == "unavailable" {
			issues = append(issues, Issue{Code: "quote_not_current", Detail: leg.ID + " is " + leg.DataStatus + " and cannot be treated as a bookable fare"})
		}
		depart, departErr := time.Parse(time.RFC3339, leg.Depart)
		arrive, arriveErr := time.Parse(time.RFC3339, leg.Arrive)
		if departErr == nil && arriveErr == nil && !arrive.After(depart) {
			issues = append(issues, Issue{Code: "impossible_timing", Detail: leg.ID + " arrives before it departs"})
		}
	}
	issues = append(issues, stayIssues(result)...)
	issues = append(issues, connectionIssues(result.Legs)...)
	if issues == nil {
		return []Issue{}
	}
	return issues
}

func stayIssues(result Result) []Issue {
	var issues []Issue
	if len(result.Stays) == 0 {
		return []Issue{{Code: "missing_hotel_night", Detail: "the trip has no hotel nights"}}
	}
	if len(result.Legs) > 0 && result.Legs[0].From != result.OriginCode {
		issues = append(issues, Issue{Code: "date_conflict", Detail: "the first ride does not leave from the origin"})
	}
	for i := 1; i < len(result.Legs); i++ {
		if result.Legs[i].From != result.Legs[i-1].To {
			issues = append(issues, Issue{Code: "date_conflict", Detail: result.Legs[i].ID + " does not continue from " + result.Legs[i-1].ID})
		}
	}
	for i, stay := range result.Stays {
		checkIn, errIn := time.Parse("2006-01-02", stay.CheckIn)
		checkOut, errOut := time.Parse("2006-01-02", stay.CheckOut)
		if errIn != nil || errOut != nil || !checkOut.After(checkIn) {
			issues = append(issues, Issue{Code: "date_conflict", Detail: stay.Name + " has dates that do not form a stay"})
			continue
		}
		nights := int(checkOut.Sub(checkIn).Hours() / 24)
		if nights != stay.Nights {
			issues = append(issues, Issue{
				Code:   "missing_hotel_night",
				Detail: fmt.Sprintf("%s covers %d nights and the stay says %d", stay.Name, nights, stay.Nights),
			})
		}
		if i > 0 {
			previous, err := time.Parse("2006-01-02", result.Stays[i-1].CheckOut)
			if err == nil && checkIn.Before(previous) {
				issues = append(issues, Issue{Code: "date_conflict", Detail: stay.Name + " overlaps the previous hotel"})
			}
			if err == nil && checkIn.After(previous) {
				issues = append(issues, Issue{Code: "missing_hotel_night", Detail: "a night between hotels has no room"})
			}
		}
		if _, ok := arrivingLeg(result.Legs, stay.City, stay.CheckIn); !ok {
			issues = append(issues, Issue{Code: "date_conflict", Detail: "arrival in " + stay.CityName + " does not match the hotel check-in"})
		}
	}
	last := result.Stays[len(result.Stays)-1]
	if len(result.Legs) > 0 {
		returnLeg := result.Legs[len(result.Legs)-1]
		if returnLeg.To != result.OriginCode || !leavesOn(result.Legs, last.City, last.CheckOut) {
			issues = append(issues, Issue{Code: "date_conflict", Detail: "the ride home does not leave on the checkout day"})
		}
	}
	return issues
}

func arrivingLeg(legs []Leg, city, date string) (Leg, bool) {
	var found Leg
	ok := false
	for _, leg := range legs {
		if leg.To == city && leg.Date == date {
			found = leg
			ok = true
		}
	}
	return found, ok
}

func leavesOn(legs []Leg, city, date string) bool {
	for _, leg := range legs {
		if leg.From == city && leg.Date == date {
			return true
		}
	}
	return false
}

func connectionIssues(legs []Leg) []Issue {
	var issues []Issue
	for i := 1; i < len(legs); i++ {
		arrive, errA := time.Parse(time.RFC3339, legs[i-1].Arrive)
		depart, errD := time.Parse(time.RFC3339, legs[i].Depart)
		if errA != nil || errD != nil {
			issues = append(issues, Issue{Code: "date_conflict", Detail: "a ride is missing its departure or arrival time"})
			continue
		}
		need := changeStationBuffer
		if legs[i-1].ToStation != "" && legs[i-1].ToStation == legs[i].FromStation {
			need = sameStationBuffer
		}
		gap := depart.Sub(arrive)
		if gap < need {
			issues = append(issues, Issue{
				Code: "connection_too_tight",
				Detail: fmt.Sprintf("%s arrives %s and %s departs %s, which leaves %s against a %s transfer",
					legs[i-1].ID, arrive.Format(time.RFC3339), legs[i].ID, depart.Format(time.RFC3339), gap, need),
			})
		}
	}
	return issues
}
