package httpapi

import (
	"testing"
	"time"

	"dr600ab-api/internal/model"
)

func TestScreenDetectionCapabilityStatusUsesUnconfiguredState(t *testing.T) {
	status := screenDetectionCapabilityStatus(
		model.DetectionSessionRequest{},
		false,
		model.DetectionSessionResponse{State: "inactive"},
	)

	if status.Configured || status.Active || status.State != "unconfigured" {
		t.Fatalf("status = %+v, want unconfigured inactive detection", status)
	}
}

func TestScreenDetectionCapabilityStatusKeepsConfiguredOfflineState(t *testing.T) {
	status := screenDetectionCapabilityStatus(
		model.DetectionSessionRequest{
			RxPortName: "/dev/rx",
			TxPortName: "/dev/tx",
		},
		true,
		model.DetectionSessionResponse{
			State:     "connecting",
			LastError: "open /dev/rx: no such file",
		},
	)

	if !status.Configured || status.Active || status.State != "connecting" {
		t.Fatalf("status = %+v, want configured offline detection", status)
	}
	if status.RxPortName != "/dev/rx" || status.TxPortName != "/dev/tx" {
		t.Fatalf("status ports = %+v, want configured rx/tx ports", status)
	}
	if status.LastError == "" {
		t.Fatalf("lastError = %q, want configured error", status.LastError)
	}
}

func TestScreenDetectionCapabilityStatusTreatsCurrentSessionAsConfigured(t *testing.T) {
	status := screenDetectionCapabilityStatus(
		model.DetectionSessionRequest{},
		false,
		model.DetectionSessionResponse{
			Active:     true,
			State:      "connected",
			RxPortName: "/dev/rx",
			TxPortName: "/dev/tx",
		},
	)

	if !status.Configured || !status.Active || status.State != "connected" {
		t.Fatalf("status = %+v, want configured active detection", status)
	}
	if status.RxPortName != "/dev/rx" || status.TxPortName != "/dev/tx" {
		t.Fatalf("status ports = %+v, want session rx/tx ports", status)
	}
}

func TestScreenDeceptionCapabilityStatusUsesUnconfiguredState(t *testing.T) {
	status := screenDeceptionCapabilityStatus(
		model.DeceptionSessionRequest{},
		false,
		model.DeceptionSessionResponse{State: "inactive"},
	)

	if status.Configured || status.Active || status.State != "unconfigured" {
		t.Fatalf("status = %+v, want unconfigured inactive deception", status)
	}
}

func TestScreenCompassCapabilityStatusUsesUnconfiguredState(t *testing.T) {
	status := screenCompassCapabilityStatus(
		model.CompassSessionRequest{},
		false,
		model.CompassSessionResponse{State: "inactive"},
	)

	if status.Configured || status.Active || status.State != "unconfigured" {
		t.Fatalf("status = %+v, want unconfigured inactive compass", status)
	}
}

func TestScreenCompassCapabilityStatusKeepsConfiguredOfflineState(t *testing.T) {
	status := screenCompassCapabilityStatus(
		model.CompassSessionRequest{PortName: "/dev/ttyUSB3"},
		true,
		model.CompassSessionResponse{
			State:     "connecting",
			LastError: "open /dev/ttyUSB3: no such file",
		},
	)

	if !status.Configured || status.Active || status.State != "connecting" {
		t.Fatalf("status = %+v, want configured offline compass", status)
	}
	if status.PortName != "/dev/ttyUSB3" || status.LastError == "" {
		t.Fatalf("status = %+v, want configured port and error", status)
	}
}

func TestScreenCompassCapabilityStatusTreatsCurrentSessionAsConfigured(t *testing.T) {
	heading := 123.45
	updatedAt := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	status := screenCompassCapabilityStatus(
		model.CompassSessionRequest{},
		false,
		model.CompassSessionResponse{
			Active:        true,
			State:         "connected",
			PortName:      "/dev/ttyUSB3",
			LastHeading:   &heading,
			LastUpdatedAt: &updatedAt,
		},
	)

	if !status.Configured || !status.Active || status.State != "connected" || status.PortName != "/dev/ttyUSB3" {
		t.Fatalf("status = %+v, want configured active compass", status)
	}
	if status.HeadingDeg == nil || *status.HeadingDeg != heading {
		t.Fatalf("heading = %v, want %.2f", status.HeadingDeg, heading)
	}
	if status.HeadingUpdatedAt == nil || !status.HeadingUpdatedAt.Equal(updatedAt) {
		t.Fatalf("heading updated at = %v, want %v", status.HeadingUpdatedAt, updatedAt)
	}
}

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
		{Latitude: 0, Longitude: 0},
	} {
		if validGeoPoint(point) {
			t.Fatalf("validGeoPoint(%+v) = true, want false", point)
		}
	}
}
