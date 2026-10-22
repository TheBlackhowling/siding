package cli

import (
	"context"
	"reflect"
	"testing"

	"github.com/TheBlackhowling/siding/internal/contract"
)

func TestHTTPEntryKeysFromContract(t *testing.T) {
	c := &contract.Contract{
		HTTPHosts: map[string][]string{"web": {"app"}},
		Modes: map[string]contract.Mode{
			"test": {Ports: map[string]int{"aliases": 3000, "api": 8000}},
		},
	}
	got := httpEntryKeysFromContract(c)
	want := []string{"web", "api"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}

	folded := httpEntryKeysFromContract(&contract.Contract{
		Modes: map[string]contract.Mode{
			"test": {Ports: map[string]int{"frontend": 3000, "backend": 8000}},
		},
	})
	if !reflect.DeepEqual(folded, want) {
		t.Fatalf("fold %#v want %#v", folded, want)
	}
}

func TestPickPrimaryBranchDefaultsToMain(t *testing.T) {
	setTTY(t, false)
	got, err := pickPrimaryBranch(context.Background(), t.TempDir(), initOpts{}, false)
	if err != nil || got != contract.DefaultPrimaryBranch {
		t.Fatalf("default = %q err = %v", got, err)
	}
}

func TestPickPrimaryBranchFlagWins(t *testing.T) {
	setTTY(t, false)
	got, err := pickPrimaryBranch(context.Background(), t.TempDir(), initOpts{primaryBranch: "develop"}, false)
	if err != nil || got != "develop" {
		t.Fatalf("flag = %q err = %v", got, err)
	}
}
