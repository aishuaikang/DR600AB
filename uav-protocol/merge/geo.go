package merge

import (
	"math"

	"uav-protocol/model"
)

type Relations struct {
	PilotDistanceM     *float64
	DroneDistanceM     *float64
	DroneDirectionDeg  *float64
	DeviceDirectionDeg *float64
}

func PositionRelations(device, drone, pilot *model.Point) Relations {
	var rel Relations
	if !ValidPoint(device) {
		return rel
	}
	if ValidPoint(drone) {
		distance, direction := DistanceAndDirectionMeters(*device, *drone)
		rel.DroneDistanceM = float64Ptr(distance)
		rel.DroneDirectionDeg = float64Ptr(direction)
		rel.DeviceDirectionDeg = float64Ptr(normalizeDegrees(direction + 180))
	}
	if ValidPoint(pilot) {
		distance, _ := DistanceAndDirectionMeters(*device, *pilot)
		rel.PilotDistanceM = float64Ptr(distance)
	}
	return rel
}

func ValidPoint(point *model.Point) bool {
	return point != nil &&
		finiteFloat64(point.Latitude) &&
		finiteFloat64(point.Longitude) &&
		point.Latitude >= -90 &&
		point.Latitude <= 90 &&
		point.Longitude >= -180 &&
		point.Longitude <= 180 &&
		!(point.Latitude == 0 && point.Longitude == 0)
}

func DistanceMeters(from, to model.Point) float64 {
	distance, _ := DistanceAndDirectionMeters(from, to)
	return distance
}

func DistanceAndDirectionMeters(from, to model.Point) (distanceM, directionDeg float64) {
	const earthRadiusM = 6_371_000

	lat1 := degreesToRadians(from.Latitude)
	lat2 := degreesToRadians(to.Latitude)
	deltaLat := degreesToRadians(to.Latitude - from.Latitude)
	deltaLon := degreesToRadians(to.Longitude - from.Longitude)
	value := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	distanceM = earthRadiusM * 2 * math.Atan2(math.Sqrt(value), math.Sqrt(1-value))

	y := math.Sin(deltaLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(deltaLon)
	directionDeg = normalizeDegrees(radiansToDegrees(math.Atan2(y, x)))
	return distanceM, directionDeg
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}

func radiansToDegrees(value float64) float64 {
	return value * 180 / math.Pi
}

func normalizeDegrees(value float64) float64 {
	if !finiteFloat64(value) {
		return 0
	}
	normalized := math.Mod(value, 360)
	if normalized < 0 {
		normalized += 360
	}
	return normalized
}

func finiteFloat64(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func float64Ptr(value float64) *float64 {
	return &value
}
