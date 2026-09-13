package id

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	a := New(Account)
	b := New(Account)
	if a == b {
		t.Fatal("expected unique ids")
	}
	if !strings.HasPrefix(a, "acc_") {
		t.Fatalf("prefix: %s", a)
	}
	if !Valid(Account, a) {
		t.Fatalf("invalid id %s", a)
	}
	if Valid(Profile, a) {
		t.Fatal("account id should not validate as profile")
	}
}
