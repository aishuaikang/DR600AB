package merge

import (
	"testing"
	"time"

	"uav-protocol/model"
)

func TestDetectionMatches(t *testing.T) {
	tests := []struct {
		name        string
		targetModel string
		targetFreq  float64
		recordModel string
		recordFreq  float64
		want        bool
	}{
		{name: "same model base threshold", targetModel: "DJI Mini", targetFreq: 2400, recordModel: "DJI Mini", recordFreq: 2410, want: true},
		{name: "autel relaxed threshold", targetModel: "Autel_type1", targetFreq: 5700, recordModel: "Autel_type3", recordFreq: 5735, want: true},
		{name: "o3 exact model relaxed threshold", targetModel: "O3+_ofdm_datalink", targetFreq: 5730, recordModel: "O3+_ofdm_datalink", recordFreq: 5750, want: true},
		{name: "o3 different model rejected", targetModel: "O3+_ofdm_datalink", targetFreq: 5730, recordModel: "DJI Mini", recordFreq: 5735, want: false},
		{name: "dji family", targetModel: "DJI_OC123_10M", targetFreq: 2400, recordModel: "DJI_OC123_20M", recordFreq: 2410, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectionMatches(tt.targetModel, tt.targetFreq, tt.recordModel, tt.recordFreq, DetectionOptions{})
			if got != tt.want {
				t.Fatalf("DetectionMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSerialMatches(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		incoming string
		want     bool
	}{
		{name: "exact", existing: "abc-123", incoming: "ABC123", want: true},
		{name: "rid prefix", existing: "1581ABCDEF123456", incoming: "ABCDEF123456", want: true},
		{name: "suffix with corrupted prefix", existing: "-XXD1234567890", incoming: "YYD1234567890", want: true},
		{name: "different", existing: "ABCDEF123456", incoming: "ZZZDEF999999", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SerialMatches(tt.existing, tt.incoming)
			if got != tt.want {
				t.Fatalf("SerialMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPositionMatchesByPendingEncryptedCorrelationID(t *testing.T) {
	existing := model.PositionTarget{Serial: "447e5681", CorrelationID: DIDEncryptedCorrelationID("447e5681")}
	incoming := model.PositionTarget{Serial: "real-sn", CorrelationID: DIDEncryptedCorrelationID("447e5681"), Cracked: true}
	if !PositionMatches(existing, incoming) {
		t.Fatal("expected pending encrypted and cracked target to match by correlation id")
	}
}

func TestPositionRelationsCalculatesDistances(t *testing.T) {
	device := &model.Point{Latitude: 31.199, Longitude: 121.399}
	drone := &model.Point{Latitude: 31.2, Longitude: 121.4}
	pilot := &model.Point{Latitude: 31.201, Longitude: 121.401}

	relations := PositionRelations(device, drone, pilot)
	if relations.DroneDistanceM == nil || *relations.DroneDistanceM <= 0 {
		t.Fatalf("drone distance = %#v, want positive", relations.DroneDistanceM)
	}
	if relations.PilotDistanceM == nil || *relations.PilotDistanceM <= 0 {
		t.Fatalf("pilot distance = %#v, want positive", relations.PilotDistanceM)
	}
	if relations.DroneDirectionDeg == nil || relations.DeviceDirectionDeg == nil {
		t.Fatalf("directions = %+v, want drone and reverse device directions", relations)
	}
}

func TestAppendTrajectoryMergesJitterAndKeepsMovement(t *testing.T) {
	base := time.Unix(1700000000, 0)
	speed := 8.5
	height := 30.0
	points := AppendTrajectory(nil, &model.Point{Latitude: 31.2, Longitude: 121.4}, base, &speed, &height, TrajectoryOptions{})
	points = AppendTrajectory(points, &model.Point{Latitude: 31.20002, Longitude: 121.4}, base.Add(time.Second), nil, nil, TrajectoryOptions{})
	if len(points) != 1 {
		t.Fatalf("trajectory count = %d, want jitter merged 1: %#v", len(points), points)
	}
	if points[0].Speed == nil || *points[0].Speed != speed {
		t.Fatalf("speed = %#v, want carried previous speed", points[0].Speed)
	}
	points = AppendTrajectory(points, &model.Point{Latitude: 31.20007, Longitude: 121.4}, base.Add(2*time.Second), nil, nil, TrajectoryOptions{})
	if len(points) != 2 {
		t.Fatalf("trajectory count = %d, want normal movement appended: %#v", len(points), points)
	}
}

func TestAppendTrajectoryRestartsOnGPSJump(t *testing.T) {
	base := time.Unix(1700000000, 0)
	points := AppendTrajectory(nil, &model.Point{Latitude: 31.2, Longitude: 121.4}, base, nil, nil, TrajectoryOptions{})
	points = AppendTrajectory(points, &model.Point{Latitude: 31.21, Longitude: 121.4}, base.Add(time.Second), nil, nil, TrajectoryOptions{})
	if len(points) != 1 {
		t.Fatalf("trajectory count = %d, want restarted 1: %#v", len(points), points)
	}
	if points[0].Latitude != 31.21 || points[0].Longitude != 121.4 {
		t.Fatalf("trajectory point = %#v, want jump point as new start", points[0])
	}
}
