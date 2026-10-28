// Package provider normalizes flight offers into one schema.
// Sample inventory stays available when this client is disabled.
package provider

import (
	"context"
	"time"
)

type Query struct {
	Origin      string
	Destination string
	Date        string
	Travelers   int
}

// Offer is a flight quoted for the whole party, in INR, after fees.
type Offer struct {
	ID          string
	Carrier     string
	Origin      string
	Destination string
	Depart      time.Time
	Arrive      time.Time
	Total       int
	Fees        int
	Source      string
}

type Flights interface {
	Enabled() bool
	Search(ctx context.Context, query Query) ([]Offer, error)
}

type Disabled struct{}

func (Disabled) Enabled() bool { return false }

func (Disabled) Search(context.Context, Query) ([]Offer, error) { return nil, nil }
