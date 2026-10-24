package release

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestLocalServerListenFailure(t *testing.T) {
	denied := errors.New("listen denied")
	server, err := localServer(http.NotFoundHandler(), func(network, address string) (net.Listener, error) {
		if network != "tcp" || address != "127.0.0.1:0" {
			t.Fatalf("unexpected listener %s %s", network, address)
		}
		return nil, denied
	})
	if server != nil || !errors.Is(err, denied) || !errors.Is(err, ErrLocalPort) || !strings.Contains(err.Error(), "no local port could be opened") {
		t.Fatalf("server=%v error=%v", server, err)
	}
}

func TestAuditListenFailureIsOneFinding(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LFM_CHROME", executable)
	result := audit(context.Background(), Target{Page: []byte("<!doctype html><title>Fixture</title>"), Assets: nil}, t.TempDir(), func(string, string) (net.Listener, error) { return nil, errors.New("listen denied") })
	if len(result.Diagnostics) != 1 {
		t.Fatalf("findings: %+v", result.Diagnostics)
	}
	finding := result.Diagnostics[0]
	if finding.Severity != "error" || finding.Rule != "release.browser" || !strings.Contains(finding.Message, "no local port could be opened") || !strings.Contains(finding.Hint, "127.0.0.1") {
		t.Fatalf("finding: %+v", finding)
	}
}
