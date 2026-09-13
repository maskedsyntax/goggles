package schedule

import (
	"testing"
	"time"
)

func TestParseAndDue(t *testing.T) {
	_, _, n, err := ParseClock("9:05")
	if err != nil || n != "09:05" {
		t.Fatalf("%s %v", n, err)
	}
	loc := time.UTC
	now := time.Date(2026, 9, 14, 12, 5, 0, 0, loc)
	occ, due, err := Due(now, loc, "12:00", nil, 15*time.Minute)
	if err != nil || !due {
		t.Fatalf("due %v %v occ=%s", due, err, occ)
	}
	last := occ.UTC()
	_, due, err = Due(now, loc, "12:00", &last, 15*time.Minute)
	if err != nil || due {
		t.Fatal("expected already fired")
	}
	_, due, err = Due(now.Add(20*time.Minute), loc, "12:00", nil, 15*time.Minute)
	if err != nil || due {
		t.Fatal("expected outside window")
	}
}

func TestNext(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, loc)
	next, err := Next(now, loc, []string{"09:00", "16:00"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Hour() != 16 || next.Day() != 14 {
		t.Fatalf("next %s", next)
	}
	next, err = Next(time.Date(2026, 9, 14, 20, 0, 0, 0, loc), loc, []string{"09:00", "16:00"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Day() != 15 || next.Hour() != 9 {
		t.Fatalf("next %s", next)
	}
}
