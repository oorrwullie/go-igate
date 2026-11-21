package igate

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/oorrwullie/sparky/internal/aprs"
)

// Geographic forwarding policy remains part of IGate; separating source files
// does not add another runtime component or change existing behavior.
func (i *IGate) allowForward(packet *aprs.Packet) bool {
	if i.geoFilter == nil {
		return true
	}

	lat, lon, ok := packet.Position()
	if !ok {
		return true
	}

	return i.geoFilter.contains(lat, lon)
}

type geoFilter struct {
	latCenter float64
	lonCenter float64
	radiusKm  float64
}

func (g *geoFilter) contains(lat, lon float64) bool {
	distance := haversineKm(g.latCenter, g.lonCenter, lat, lon)
	return distance <= g.radiusKm
}

func parseRangeFilter(filter string) (*geoFilter, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil, nil
	}

	tokens := strings.Fields(filter)
	for _, token := range tokens {
		if strings.HasPrefix(token, "r/") {
			parts := strings.Split(token[2:], "/")
			if len(parts) != 3 {
				return nil, fmt.Errorf("invalid range filter token %q", token)
			}

			lat, err := strconv.ParseFloat(parts[0], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid latitude: %w", err)
			}

			lon, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid longitude: %w", err)
			}

			radius, err := strconv.ParseFloat(parts[2], 64)
			if err != nil {
				return nil, fmt.Errorf("invalid radius: %w", err)
			}

			return &geoFilter{
				latCenter: lat,
				lonCenter: lon,
				radiusKm:  radius,
			}, nil
		}
	}

	return nil, nil
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0

	dLat := degreesToRadians(lat2 - lat1)
	dLon := degreesToRadians(lon2 - lon1)

	lat1Rad := degreesToRadians(lat1)
	lat2Rad := degreesToRadians(lat2)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

func degreesToRadians(deg float64) float64 {
	return deg * (math.Pi / 180)
}
