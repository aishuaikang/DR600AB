package httpapi

import (
	"testing"
	"time"

	"dr600ab-api/internal/model"
)

func TestScreenDeviceLocationResponsePrefersGPSOverManual(t *testing.T) {
	updatedAt := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	response := screenDeviceLocationResponse(
		&model.GPSFix{Latitude: 23.1, Longitude: 113.2, Valid: true},
		&updatedAt,
		model.UserSettings{
			ManualDeviceLocation: &model.GeoPoint{Latitude: 39.9, Longitude: 116.3},
		},
	)

	if response.Source != "gps" || !response.Valid || response.Point == nil {
		t.Fatalf("response = %+v, want GPS point", response)
	}
	if response.Point.Latitude != 23.1 || response.Point.Longitude != 113.2 {
		t.Fatalf("point = %+v, want GPS coordinates", response.Point)
	}
	if response.UpdatedAt == nil || !response.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updatedAt = %v, want %v", response.UpdatedAt, updatedAt)
	}
}

func TestScreenDeviceLocationResponseFallsBackToManual(t *testing.T) {
	response := screenDeviceLocationResponse(
		nil,
		nil,
		model.UserSettings{
			ManualDeviceLocation: &model.GeoPoint{Latitude: 39.9, Longitude: 116.3},
		},
	)

	if response.Source != "manual" || !response.Valid || response.Point == nil {
		t.Fatalf("response = %+v, want manual point", response)
	}
	if response.Point.Latitude != 39.9 || response.Point.Longitude != 116.3 {
		t.Fatalf("point = %+v, want manual coordinates", response.Point)
	}
}

func TestScreenDeviceLocationResponseReturnsNoneWithoutValidPoint(t *testing.T) {
	response := screenDeviceLocationResponse(
		&model.GPSFix{Latitude: 91, Longitude: 113.2, Valid: true},
		nil,
		model.UserSettings{},
	)

	if response.Source != "none" || response.Valid || response.Point != nil {
		t.Fatalf("response = %+v, want none", response)
	}
}

func TestValidGeoPointRejectsInvalidCoordinates(t *testing.T) {
	for _, point := range []*model.GeoPoint{
		nil,
		{Latitude: -91, Longitude: 0},
		{Latitude: 91, Longitude: 0},
		{Latitude: 0, Longitude: -181},
		{Latitude: 0, Longitude: 181},
	} {
		if validGeoPoint(point) {
			t.Fatalf("validGeoPoint(%+v) = true, want false", point)
		}
	}

	if !validGeoPoint(&model.GeoPoint{Latitude: 0, Longitude: 0}) {
		t.Fatal("validGeoPoint(0,0) = false, want true")
	}
}
