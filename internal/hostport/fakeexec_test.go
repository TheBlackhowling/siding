package hostport

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestWindowsAndUnixWithFakeRun(t *testing.T) {
	t.Cleanup(func() { cmdRun = execRun })
	cmdRun = func(_ context.Context, name string, args ...string) (string, error) {
		switch name {
		case "netstat":
			return "  TCP    0.0.0.0:80    0.0.0.0:0    LISTENING    42\n", nil
		case "tasklist":
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "/FO CSV /NH") {
				return "", fmt.Errorf("must ask for headerless CSV: %s", joined)
			}
			if strings.HasSuffix(joined, "PID eq 42") {
				return `"caddy.exe","42","Services","0","12,345 K"` + "\n", nil
			}
			return "INFO: No tasks are running which match the specified criteria.\n", nil
		case "netsh":
			return "Start Port    End Port\n----------    --------\n        80          80     *\n\n* - Administered port exclusions.\n", nil
		case "ss":
			return `LISTEN 0 128 0.0.0.0:80 0.0.0.0:* users:(("caddy",pid=42,fd=7))` + "\n", nil
		case "lsof":
			return "caddy    42 nobody   IPv4  TCP *:80 (LISTEN)\n", nil
		default:
			return "", fmt.Errorf("unknown %s", name)
		}
	}
	ctx := context.Background()
	win := windowsOccupants(ctx, 80)
	if len(win) != 1 || win[0].PID != 42 || win[0].Name != "caddy.exe" {
		t.Fatalf("windows %#v", win)
	}
	if name := windowsProcessName(ctx, 42); name != "caddy.exe" {
		t.Fatalf("name %q", name)
	}
	if name := windowsProcessName(ctx, 99); name != "" {
		t.Fatalf("unknown pid must resolve to empty, got %q", name)
	}
	excl := windowsExcluded(ctx, 80)
	if len(excl) != 1 || excl[0].Source != "windows" || !strings.Contains(excl[0].Detail, "TCP 80-80") {
		t.Fatalf("administered (*) exclusion must be found: %#v", excl)
	}
	if excl := windowsExcluded(ctx, 443); excl != nil {
		t.Fatalf("443 %#v", excl)
	}
	unix := unixOccupants(ctx, 80)
	if len(unix) != 1 || unix[0].Name != "caddy" {
		t.Fatalf("unix %#v", unix)
	}
	cmdRun = func(_ context.Context, name string, _ ...string) (string, error) {
		if name == "ss" {
			return "", fmt.Errorf("no ss")
		}
		if name == "lsof" {
			return "caddy    7 nobody   IPv4  TCP *:80 (LISTEN)\n", nil
		}
		return "", fmt.Errorf("no")
	}
	unix = unixOccupants(ctx, 80)
	if len(unix) != 1 || unix[0].PID != 7 {
		t.Fatalf("lsof fallback %#v", unix)
	}
}
