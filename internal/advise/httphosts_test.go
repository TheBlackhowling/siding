package advise

import (
	"testing"

	"github.com/TheBlackhowling/siding/internal/contract"
)

func TestInferHTTPHostFromComposeDefaults(t *testing.T) {
	key, label := InferHTTPHost("NEXT_PUBLIC_ROOT_DOMAIN", "${NEXT_PUBLIC_ROOT_DOMAIN:-app.acme.test:3000}")
	if key != "web" || label != "app" {
		t.Fatalf("root domain → %s %s", key, label)
	}
	key, label = InferHTTPHost("DEV_FRONTEND_URL", "http://www.example.test:3000")
	if key != "web" || label != "www" {
		t.Fatalf("frontend → %s %s", key, label)
	}
	key, label = InferHTTPHost("DEV_API_URL", "http://api.acme.test:8000")
	if key != "api" || label != "api" {
		t.Fatalf("api → %s %s", key, label)
	}
	key, label = InferHTTPHost("NEXT_PUBLIC_ROOT_DOMAIN", "acme.test")
	if key != "" {
		t.Fatalf("apex should not invent a subdomain: %s %s", key, label)
	}
}

func TestInferHostPrefixFromComposeDefault(t *testing.T) {
	if got := contract.InferHostPrefixFromHostname(hostnameFromValue("${NEXT_PUBLIC_ROOT_DOMAIN:-app.acme.test:3000}")); got != "acme" {
		t.Fatalf("got %q", got)
	}
	if got := contract.InferHostPrefixFromHostname(hostnameFromValue("http://hello.test:3000")); got != "" {
		t.Fatalf("single-label leftover %q", got)
	}
}

func TestInferHTTPHostsFromService(t *testing.T) {
	key, label := InferHTTPHostsFromService("app")
	if key != "web" || label != "app" {
		t.Fatalf("app service → %s %s", key, label)
	}
	key, label = InferHTTPHostsFromService("frontend")
	if key != "web" || label != "frontend" {
		t.Fatalf("frontend service → %s %s", key, label)
	}
	if k, _ := InferHTTPHostsFromService("postgres"); k != "" {
		t.Fatal("postgres is not an http host")
	}
}
