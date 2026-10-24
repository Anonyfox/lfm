package release

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// Target selects a live URL or immutable artifact bytes for browser checks.
type Target struct {
	URL    string
	Page   []byte
	Assets map[string][]byte
}

func (t Target) serve(listen func(string, string) (net.Listener, error)) (destination, proxy string, closeTarget func(), err error) {
	if t.URL != "" {
		return t.URL, "", func() {}, nil
	}
	server, err := localServerWithOrigin(func(origin string) http.Handler { return AuditHandler(t.Page, t.Assets, origin) }, listen)
	if err != nil {
		return "", "", func() {}, err
	}
	return server.URL + "/", server.URL, server.Close, nil
}

// toolRoot is the directory used to discover local audit tools.
func (t Target) toolRoot(root string) string {
	if t.URL != "" {
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return "."
		}
	}
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		return filepath.Dir(root)
	}
	return root
}

func (t Target) auditScope() string {
	if t.URL != "" {
		return "Live URL with application JavaScript; host delivery checks are reported separately."
	}
	return auditScope
}
