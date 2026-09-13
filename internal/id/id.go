package id

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	Account     = "acc"
	Destination = "dst"
	Profile     = "prf"
	Schedule    = "sch"
	Queue       = "que"
	Batch       = "batch"
	Job         = "job"
	Publication = "pub"
	Upload      = "upl"
	Event       = "evt"
)

func New(prefix string) string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now().UTC()), entropy)
	return prefix + "_" + strings.ToLower(id.String())
}

func Valid(prefix, value string) bool {
	want := prefix + "_"
	if !strings.HasPrefix(value, want) {
		return false
	}
	_, err := ulid.Parse(strings.TrimPrefix(value, want))
	return err == nil
}

func MustPrefix(prefix, value string) error {
	if !Valid(prefix, value) {
		return fmt.Errorf("invalid %s id", prefix)
	}
	return nil
}
