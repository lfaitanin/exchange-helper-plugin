package main

import (
	"googlemaps.github.io/maps"
	"testing"
)

func TestExtractPriceValue(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"€1,234", 1234},
		{"1,234", 1234},
		{"1 234", 1234},
	}
	for _, c := range cases {
		got := extractPriceValue(c.input)
		if got != c.expected {
			t.Errorf("extractPriceValue(%q) = %v, want %v", c.input, got, c.expected)
		}
	}
}

func TestFindPublicTransport_EmptyTypes(t *testing.T) {
	property := &PropertyInfo{}
	property.Coordinates.Lat = 0
	property.Coordinates.Lng = 0

	// Stub searchNearbyPlaces
	searchNearbyPlacesFn = func(client *maps.Client, location *maps.LatLng, placeType string, radius uint) ([]maps.PlacesSearchResult, error) {
		res := maps.PlacesSearchResult{Name: "Test Station"}
		res.Geometry.Location = maps.LatLng{Lat: 0.1, Lng: 0.1}
		// Types left empty
		return []maps.PlacesSearchResult{res}, nil
	}
	defer func() { searchNearbyPlacesFn = searchNearbyPlaces }()

	if err := findPublicTransport(property, &maps.Client{}); err != nil {
		t.Fatalf("findPublicTransport returned error: %v", err)
	}

	if len(property.QualityOfLife.PublicTransport) != 1 {
		t.Fatalf("expected 1 transport, got %d", len(property.QualityOfLife.PublicTransport))
	}
	if property.QualityOfLife.PublicTransport[0].Type != "" {
		t.Fatalf("expected empty type, got %q", property.QualityOfLife.PublicTransport[0].Type)
	}
}
