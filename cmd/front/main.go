package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

// placeholderIndex is the frontend shell the standalone front serves. It only
// talks to the hub through same-origin /api, /saml and /ws requests, which this
// proxy forwards server-side to the hub FQDN, so no hub address is ever baked
// into the page. A real SPA bundle replaces it later.
// ponytail: SSO lives on the hub (/saml); the shell never handles credentials.
const placeholderIndex = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>kubeshark</title>
</head>
<body>
  <main>
    <h1>kubeshark</h1>
    <p id="status">Loading hub info…</p>
  </main>
  <script>
    fetch('/api/metadata/version')
      .then(function (r) { return r.json(); })
      .then(function (v) {
        document.getElementById('status').textContent =
          'hub version ' + v.ver + (v.hubUrl ? ' (' + v.hubUrl + ')' : '');
      })
      .catch(function (e) {
        document.getElementById('status').textContent = 'hub unreachable: ' + e;
      });
  </script>
</body>
</html>
`

// defaultHubURL is the hub Service's cluster FQDN; override per namespace with
// HUB_URL or -hub-url.
func defaultHubURL() string {
	if v := os.Getenv("HUB_URL"); v != "" {
		return v
	}
	return "http://kubeshark-hub.default.svc.cluster.local"
}

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	hubURL := flag.String("hub-url", defaultHubURL(), "hub base URL to proxy /api, /saml and /ws to")
	flag.Parse()

	hubBase, err := url.Parse(*hubURL)
	if err != nil {
		log.Fatalf("invalid -hub-url %q: %v", *hubURL, err)
	}
	log.Printf("kubeshark front serving :%d and proxying to hub %s", *port, hubBase)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           newHandler(hubBase),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

// newHandler proxies API, SAML and WebSocket paths to the hub and serves the
// SPA shell for everything else. /api is prefix-stripped, matching the chart's
// old nginx default.conf so the hub sees its own route paths.
func newHandler(hubBase *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(hubBase)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api"):
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
			proxy.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/saml"),
			strings.HasPrefix(r.URL.Path, "/ws"),
			strings.HasPrefix(r.URL.Path, "/debug/pprof"):
			proxy.ServeHTTP(w, r)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, placeholderIndex)
		}
	})
}