package release

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
)

// LocalPortSetup explains how to let Chrome load pages from lfm.
const LocalPortSetup = "Run lfm where it may listen on 127.0.0.1 (a sandbox may block local ports), then retry."

// ErrLocalPort means no loopback port could be opened for Chrome to load from.
var ErrLocalPort = errors.New("no local port could be opened for Chrome to load the page from")

// localServer reserves the listener before starting HTTP so unavailable ports
// return through the release stage's operational error path.
func localServer(handler http.Handler, listen func(string, string) (net.Listener, error)) (*httptest.Server, error) {
	return localServerWithOrigin(func(string) http.Handler { return handler }, listen)
}

func localServerWithOrigin(handler func(string) http.Handler, listen func(string, string) (net.Listener, error)) (*httptest.Server, error) {
	listener, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLocalPort, err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler("http://" + listener.Addr().String())}}
	server.Start()
	return server, nil
}
