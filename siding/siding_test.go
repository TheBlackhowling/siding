package siding_test

import (
	"testing"

	"github.com/TheBlackhowling/siding/siding"
)

func TestNewClient(t *testing.T) {
	c := siding.New("")
	if c == nil {
		t.Fatal("New returned nil")
	}
}
