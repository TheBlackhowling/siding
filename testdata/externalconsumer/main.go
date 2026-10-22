// Compile-only stub: AEOS (or any external module) imports the public API.
package main

import (
	"context"

	"github.com/TheBlackhowling/siding/siding"
)

func main() {
	client := siding.New("")
	target := siding.Target{Mode: "test"}
	_, _ = client.Plan(context.Background(), target)
}
