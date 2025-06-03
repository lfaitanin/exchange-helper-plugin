package main

import (
	"googlemaps.github.io/maps"
	"testing"
)

func TestStationToPOIUnknownType(t *testing.T) {
	origin := &maps.LatLng{Lat: 0, Lng: 0}
	station := maps.PlacesSearchResult{
		Name:     "Test Station",
		Geometry: maps.AddressGeometry{Location: maps.LatLng{Lat: 0, Lng: 0}},
		Types:    []string{},
	}

	poi := stationToPOI(station, origin)
	if poi.Type != "unknown" {
		t.Errorf("expected type 'unknown', got %s", poi.Type)
	}
}
