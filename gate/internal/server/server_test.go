package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthProvesTheSamplePlanner(t *testing.T) {
	handler := New()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["service"] != "portage-gate" || body["self_check"] != "passed" || body["flight_provider"] != "sample" {
		t.Fatalf("%s", recorder.Body.String())
	}
	if body["self_check_total"] != float64(50540) {
		t.Fatalf("total %v", body["self_check_total"])
	}
}

func TestPlanAndBadJSONAreObservable(t *testing.T) {
	handler := New()
	plan := httptest.NewRecorder()
	handler.ServeHTTP(plan, httptest.NewRequest(http.MethodPost, "/v1/plan", strings.NewReader(
		`{"origin":"DEL","start_date":"2026-12-10","travelers":2,"budget":80000,"stops":[{"city":"JAI","nights":2},{"city":"BOM","nights":2}]}`,
	)))
	if plan.Code != http.StatusOK {
		t.Fatalf("plan %d %s", plan.Code, plan.Body.String())
	}
	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/v1/plan", strings.NewReader(`{`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad %d", bad.Code)
	}
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	var body map[string]any
	if err := json.Unmarshal(health.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["plans"] != float64(1) || body["request_errors"] != float64(1) || body["last_error"] != "invalid JSON" {
		t.Fatalf("%s", health.Body.String())
	}
}
