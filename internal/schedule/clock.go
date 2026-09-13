package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/maskedsyntax/goggles/internal/apperr"
)

func ParseClock(s string) (hour, minute int, norm string, err error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, "", apperr.Invalid("schedule times must be HH:MM")
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, "", apperr.Invalid("invalid hour in " + s)
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, "", apperr.Invalid("invalid minute in " + s)
	}
	return hour, minute, fmt.Sprintf("%02d:%02d", hour, minute), nil
}

func Occurrence(now time.Time, loc *time.Location, hhmm string) (time.Time, error) {
	h, m, _, err := ParseClock(hhmm)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), h, m, 0, 0, loc), nil
}

func Due(now time.Time, loc *time.Location, hhmm string, lastFired *time.Time, window time.Duration) (time.Time, bool, error) {
	occ, err := Occurrence(now, loc, hhmm)
	if err != nil {
		return time.Time{}, false, err
	}
	if now.Before(occ) {
		return occ, false, nil
	}
	if now.Sub(occ) > window {
		return occ, false, nil
	}
	if lastFired != nil && lastFired.UTC().Equal(occ.UTC()) {
		return occ, false, nil
	}
	return occ, true, nil
}

func Next(now time.Time, loc *time.Location, slots []string) (time.Time, error) {
	if len(slots) == 0 {
		return time.Time{}, apperr.Invalid("no schedule slots")
	}
	var best time.Time
	for _, s := range slots {
		occ, err := Occurrence(now, loc, s)
		if err != nil {
			return time.Time{}, err
		}
		if !occ.After(now) {
			occ = occ.Add(24 * time.Hour)
		}
		if best.IsZero() || occ.Before(best) {
			best = occ
		}
	}
	return best, nil
}

func LoadTZ(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, apperr.Invalid("unknown timezone " + name)
	}
	return loc, nil
}
