package model

import "math"

func ScreenPositionRelations(deviceLocation *ScreenDeviceLocationResponse, drone, pilot *ScreenPositionPoint) (pilotDistanceM, droneDistanceM, droneDirectionDeg, deviceDirectionDeg *float64) {
	if deviceLocation == nil || !deviceLocation.Valid || !validGeoPoint(deviceLocation.Point) {
		return nil, nil, nil, nil
	}

	devicePoint := *deviceLocation.Point
	if point, ok := screenPositionPointToGeoPoint(drone); ok {
		distance, direction := geoDistanceAndDirection(devicePoint, point)
		reverseDirection := normalizeDegrees(direction + 180)
		droneDistanceM = float64Ptr(distance)
		droneDirectionDeg = float64Ptr(direction)
		deviceDirectionDeg = float64Ptr(reverseDirection)
	}
	if point, ok := screenPositionPointToGeoPoint(pilot); ok {
		distance, _ := geoDistanceAndDirection(devicePoint, point)
		pilotDistanceM = float64Ptr(distance)
	}
	return pilotDistanceM, droneDistanceM, droneDirectionDeg, deviceDirectionDeg
}

func screenPositionPointToGeoPoint(point *ScreenPositionPoint) (GeoPoint, bool) {
	if point == nil {
		return GeoPoint{}, false
	}
	geoPoint := GeoPoint{
		Latitude:  point.Latitude,
		Longitude: point.Longitude,
	}
	if !validGeoPoint(&geoPoint) {
		return GeoPoint{}, false
	}
	return geoPoint, true
}

func geoDistanceAndDirection(from, to GeoPoint) (distanceM, directionDeg float64) {
	const earthRadiusM = 6_371_000

	lat1 := degreesToRadians(from.Latitude)
	lat2 := degreesToRadians(to.Latitude)
	deltaLat := degreesToRadians(to.Latitude - from.Latitude)
	deltaLon := degreesToRadians(to.Longitude - from.Longitude)
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	distanceM = earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	y := math.Sin(deltaLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(deltaLon)
	directionDeg = normalizeDegrees(radiansToDegrees(math.Atan2(y, x)))
	return distanceM, directionDeg
}

func normalizeDegrees(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	normalized := math.Mod(value, 360)
	if normalized < 0 {
		normalized += 360
	}
	return normalized
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}

func radiansToDegrees(value float64) float64 {
	return value * 180 / math.Pi
}

func float64Ptr(value float64) *float64 {
	return &value
}

func validGeoPoint(point *GeoPoint) bool {
	if point == nil {
		return false
	}
	return !math.IsNaN(point.Latitude) &&
		!math.IsInf(point.Latitude, 0) &&
		!math.IsNaN(point.Longitude) &&
		!math.IsInf(point.Longitude, 0) &&
		point.Latitude >= -90 &&
		point.Latitude <= 90 &&
		point.Longitude >= -180 &&
		point.Longitude <= 180 &&
		!(point.Latitude == 0 && point.Longitude == 0)
}
