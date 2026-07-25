package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-monitor/shared"
)

func TestSearchResourcesMatchesHardwareFields(t *testing.T) {
	s := &Server{store: NewStore(), hw: newHardwareStore()}
	s.hw.put("host-1", "edge-node-01", "10.0.0.8", []shared.HardwareSnapshot{{
		TargetName: "idrac-prod",
		System: shared.RedfishSystem{
			Manufacturer: "Dell",
			Model:        "PowerEdge R740",
			SerialNumber: "SN-ABC-123",
		},
	}})

	results := s.searchResources("r740", 20)
	if len(results) != 1 {
		t.Fatalf("expected one match, got %#v", results)
	}
	got := results[0]
	if got.Type != "hardware" || got.Name != "idrac-prod" || got.Host != "edge-node-01" ||
		got.Ref != "hardware:host-1/idrac-prod" || got.View != "hardware" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestHandleResourceSearchEmptyQueryShape(t *testing.T) {
	s := &Server{store: NewStore()}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resources/search?q=&limit=20", nil)
	s.handleResourceSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"count\":0,\"query\":\"\",\"results\":[]}\n" {
		t.Fatalf("unexpected response: %s", got)
	}
}
