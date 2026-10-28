package plan

import (
	"strings"
	"testing"
	"time"
)

func TestValidatorCatchesBrokenPlans(t *testing.T) {
	base := buildDemo(t)

	tight := base
	arrive, err := time.Parse(time.RFC3339, tight.Legs[0].Arrive)
	if err != nil {
		t.Fatal(err)
	}
	tight.Legs[1].Depart = arrive.Add(15 * time.Minute).Format(time.RFC3339)
	if !hasIssue(Validate(tight), "connection_too_tight") {
		t.Fatalf("%+v", Validate(tight))
	}

	// 18:00 IST and 13:00 UTC are 30 minutes apart, not an afternoon of slack.
	zones := base
	zones.Legs[0].Arrive = "2026-12-10T18:00:00+05:30"
	zones.Legs[1].Depart = "2026-12-10T13:00:00Z"
	zones.Legs[0].ToStation = "JAI-SINDHI"
	zones.Legs[1].FromStation = "JAI"
	if !hasIssue(Validate(zones), "connection_too_tight") {
		t.Fatalf("%+v", Validate(zones))
	}

	shortStay := base
	shortStay.Stays[0].Nights = 1
	if !hasIssue(Validate(shortStay), "missing_hotel_night") {
		t.Fatalf("%+v", Validate(shortStay))
	}

	gap := base
	gap.Stays[1].CheckIn = "2026-12-13"
	if !hasIssue(Validate(gap), "missing_hotel_night") {
		t.Fatalf("%+v", Validate(gap))
	}

	overlap := base
	overlap.Stays[1].CheckIn = "2026-12-11"
	if !hasIssue(Validate(overlap), "date_conflict") {
		t.Fatalf("%+v", Validate(overlap))
	}

	mismatch := base
	mismatch.Total++
	if !hasIssue(Validate(mismatch), "total_mismatch") {
		t.Fatalf("%+v", Validate(mismatch))
	}

	flag := base
	flag.FitsBudget = false
	if !hasIssue(Validate(flag), "budget_flag") {
		t.Fatalf("%+v", Validate(flag))
	}

	train := base
	train.Legs[0].Mode = "train"
	if !hasIssue(Validate(train), "train_marked_live") {
		t.Fatalf("%+v", Validate(train))
	}

	free := base
	free.Lines[0].Amount = 0
	free.Total -= 1700
	if !hasIssue(Validate(free), "missing_price") {
		t.Fatalf("%+v", Validate(free))
	}
}

func TestSameStationBufferAllowsNinetyMinutes(t *testing.T) {
	base := buildDemo(t)
	arrive, err := time.Parse(time.RFC3339, base.Legs[0].Arrive)
	if err != nil {
		t.Fatal(err)
	}
	base.Legs[0].ToStation = "JAI"
	base.Legs[1].FromStation = "JAI"
	base.Legs[1].Depart = arrive.Add(90 * time.Minute).Format(time.RFC3339)
	if hasIssue(Validate(base), "connection_too_tight") {
		t.Fatalf("90 minutes at the same station should pass: %+v", Validate(base))
	}
	base.Legs[1].Depart = arrive.Add(89 * time.Minute).Format(time.RFC3339)
	if !hasIssue(Validate(base), "connection_too_tight") {
		t.Fatal("89 minutes should fail")
	}
}

func hasIssue(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestIssueTextNamesTheRides(t *testing.T) {
	base := buildDemo(t)
	base.Legs[0].Arrive = "2026-12-10T18:00:00+05:30"
	base.Legs[1].Depart = "2026-12-10T13:00:00Z"
	text := Validate(base)[0].Detail
	if !strings.Contains(text, "18:00:00+05:30") || !strings.Contains(text, "13:00:00Z") {
		t.Fatal(text)
	}
}
