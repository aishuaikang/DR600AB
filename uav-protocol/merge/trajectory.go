package merge

import (
	"slices"
	"time"

	"uav-protocol/model"
)

const (
	DefaultTrajectoryJitterM = 3.0
	DefaultTrajectoryJumpM   = 500.0
)

type TrajectoryOptions struct {
	Limit   int
	JitterM float64
	JumpM   float64
}

func (o TrajectoryOptions) limit() int {
	return o.Limit
}

func (o TrajectoryOptions) jitterM() float64 {
	if o.JitterM > 0 {
		return o.JitterM
	}
	return DefaultTrajectoryJitterM
}

func (o TrajectoryOptions) jumpM() float64 {
	if o.JumpM > 0 {
		return o.JumpM
	}
	return DefaultTrajectoryJumpM
}

func AppendTrajectory(
	trajectory []model.TrackPoint,
	point *model.Point,
	seenAt time.Time,
	speed *float64,
	height *float64,
	opts TrajectoryOptions,
) []model.TrackPoint {
	result := NormalizeTrajectory(trajectory)
	if !ValidPoint(point) || seenAt.IsZero() {
		return CompactTrajectory(result, opts)
	}
	result = append(result, model.TrackPoint{
		Latitude:  point.Latitude,
		Longitude: point.Longitude,
		Speed:     cloneFloat64Ptr(speed),
		Height:    cloneFloat64Ptr(height),
		Time:      seenAt,
	})
	return CompactTrajectory(result, opts)
}

func MergeTrajectories(current []model.TrackPoint, incoming []model.TrackPoint, opts TrajectoryOptions) []model.TrackPoint {
	if len(current) == 0 {
		return CompactTrajectory(incoming, opts)
	}
	if len(incoming) == 0 {
		return CompactTrajectory(current, opts)
	}
	merged := append(NormalizeTrajectory(current), NormalizeTrajectory(incoming)...)
	return CompactTrajectory(merged, opts)
}

func CompactTrajectory(points []model.TrackPoint, opts TrajectoryOptions) []model.TrackPoint {
	return trimTrajectory(deduplicateAndRestartTrajectory(NormalizeTrajectory(points), opts), opts.limit())
}

func NormalizeTrajectory(points []model.TrackPoint) []model.TrackPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]model.TrackPoint, 0, len(points))
	for _, point := range points {
		if !validTrackPointCoordinate(point.Latitude, point.Longitude) || point.Time.IsZero() {
			continue
		}
		point.Speed = cloneFloat64Ptr(point.Speed)
		point.Height = cloneFloat64Ptr(point.Height)
		out = append(out, point)
	}
	return out
}

func deduplicateAndRestartTrajectory(points []model.TrackPoint, opts TrajectoryOptions) []model.TrackPoint {
	if len(points) <= 1 {
		return points
	}
	slices.SortFunc(points, func(a, b model.TrackPoint) int {
		if result := a.Time.Compare(b.Time); result != 0 {
			return result
		}
		if a.Latitude < b.Latitude {
			return -1
		}
		if a.Latitude > b.Latitude {
			return 1
		}
		if a.Longitude < b.Longitude {
			return -1
		}
		if a.Longitude > b.Longitude {
			return 1
		}
		return 0
	})

	out := points[:0]
	for _, point := range points {
		if len(out) > 0 {
			last := out[len(out)-1]
			distance := trackPointDistanceM(last, point)
			if distance > opts.jumpM() {
				out = out[:0]
				out = append(out, point)
				continue
			}
			if distance <= opts.jitterM() {
				out[len(out)-1] = mergeTrackPoint(last, point)
				continue
			}
		}
		out = append(out, point)
	}
	clear(points[len(out):])
	return out
}

func trimTrajectory(points []model.TrackPoint, limit int) []model.TrackPoint {
	if limit <= 0 || len(points) <= limit {
		return points
	}
	return points[len(points)-limit:]
}

func mergeTrackPoint(current, latest model.TrackPoint) model.TrackPoint {
	merged := latest
	if latest.Speed == nil {
		merged.Speed = cloneFloat64Ptr(current.Speed)
	} else {
		merged.Speed = cloneFloat64Ptr(latest.Speed)
	}
	if latest.Height == nil {
		merged.Height = cloneFloat64Ptr(current.Height)
	} else {
		merged.Height = cloneFloat64Ptr(latest.Height)
	}
	return merged
}

func trackPointDistanceM(a, b model.TrackPoint) float64 {
	return DistanceMeters(
		model.Point{Latitude: a.Latitude, Longitude: a.Longitude},
		model.Point{Latitude: b.Latitude, Longitude: b.Longitude},
	)
}

func validTrackPointCoordinate(latitude, longitude float64) bool {
	return finiteFloat64(latitude) &&
		finiteFloat64(longitude) &&
		latitude >= -90 &&
		latitude <= 90 &&
		longitude >= -180 &&
		longitude <= 180 &&
		!(latitude == 0 && longitude == 0)
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
