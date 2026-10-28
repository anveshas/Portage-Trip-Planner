package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"portage/gate/internal/catalog"
	"portage/gate/internal/plan"
	"portage/gate/internal/provider"
)

type stats struct {
	mu            sync.Mutex
	plans         int
	apiFailures   int
	requestErrors int
	lastError     string
	lastRoute     string
	lastTotal     int
	lastAt        string
}

func (s *stats) recordPlan(result plan.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans++
	s.apiFailures += result.Report.APIFailures
	s.lastRoute = result.Route
	s.lastTotal = result.Total
	s.lastAt = time.Now().Format(time.RFC3339)
	s.lastError = ""
}

func (s *stats) recordError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestErrors++
	s.lastError = message
	s.lastAt = time.Now().Format(time.RFC3339)
	log.Printf("plan error %s", message)
}

func New() http.Handler {
	inventory := catalog.Load()
	flights := provider.NewFromEnv(nil)
	providerName := "sample"
	if flights.Enabled() {
		providerName = "amadeus-test"
	}
	selfTotal, selfStatus := selfCheck(inventory)
	state := &stats{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		state.mu.Lock()
		body := map[string]any{
			"status":           selfStatus,
			"service":          "portage-gate",
			"flight_provider":  providerName,
			"inventory_rides":  len(inventory.Transports),
			"self_check":       map[bool]string{true: "passed", false: "failed"}[selfStatus == "ok"],
			"self_check_total": selfTotal,
			"plans":            state.plans,
			"api_failures":     state.apiFailures,
			"request_errors":   state.requestErrors,
			"last_error":       state.lastError,
			"last_route":       state.lastRoute,
			"last_total":       state.lastTotal,
			"last_plan_at":     state.lastAt,
		}
		state.mu.Unlock()
		status := http.StatusOK
		if selfStatus != "ok" {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, body)
	})
	mux.HandleFunc("POST /v1/plan", func(w http.ResponseWriter, r *http.Request) {
		var request plan.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			state.recordError("invalid JSON")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := plan.Build(r.Context(), inventory, flights, request)
		if err != nil {
			state.recordError(err.Error())
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		state.recordPlan(result)
		log.Printf("plan route=%q total=%d fits=%v valid=%v booked=%v adjusted=%v api_failures=%d",
			result.Route, result.Total, result.FitsBudget, result.Valid, result.Booked, result.Adjusted, result.Report.APIFailures)
		writeJSON(w, http.StatusOK, result)
	})
	return mux
}

func selfCheck(inventory catalog.Inventory) (int, string) {
	result, err := plan.Build(context.Background(), inventory, nil, plan.Request{
		Origin: "DEL", StartDate: "2026-12-10", Travelers: 2, Budget: 80000,
		Stops: []plan.Stop{{City: "JAI", Nights: 2}, {City: "BOM", Nights: 2}},
	})
	if err != nil || !result.Valid || !result.FitsBudget || result.Total != 50540 || result.Adjusted {
		log.Printf("self-check failed total=%d err=%v", result.Total, err)
		return result.Total, "degraded"
	}
	return result.Total, "ok"
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
