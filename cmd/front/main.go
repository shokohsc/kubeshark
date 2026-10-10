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
// The page shows the hub version (GET /api/metadata/version) and live traffic:
// it seeds from GET /api/flows2 and appends the hub's /ws stream. Entries are
// opaque JSON, so the columns read src/dst/protocol/http fields best-effort and
// fall back to the raw entry.
const placeholderIndex = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>kubeshark</title>
  <style>
    body { font: 13px/1.4 ui-monospace, monospace; margin: 0; }
    header { display: flex; gap: 12px; align-items: baseline; padding: 8px 12px; border-bottom: 1px solid #ddd; }
    #version { color: #666; font-size: 12px; }
    table { border-collapse: collapse; width: 100%; }
    th, td { max-width: 280px; padding: 2px 8px; text-align: left; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; border-bottom: 1px solid #eee; }
    th { position: sticky; top: 0; background: #fafafa; }
    #empty { padding: 12px; color: #888; }
  </style>
</head>
<body>
  <header>
    <strong>kubeshark traffic</strong>
    <span id="version">connecting…</span>
  </header>
  <table id="flows" hidden>
    <thead><tr><th>Time</th><th>Source</th><th>Destination</th><th>Protocol</th><th>Info</th></tr></thead>
    <tbody></tbody>
  </table>
  <p id="empty">Waiting for traffic…</p>
  <script>
  (function () {
    var MAX = 500;
    var tbody = document.querySelector('#flows tbody');
    var table = document.getElementById('flows');
    var empty = document.getElementById('empty');

    function text(v) { return v === undefined || v === null || v === '' ? '' : String(v); }

    function endpoint(o) {
      if (!o) return '';
      var name = (o.pod && o.pod.name) || (o.service && o.service.name) || o.name || text(o.ip);
      var ns = (o.pod && o.pod.namespace) || o.namespace;
      if (ns) name = name ? name + '.' + ns : ns;
      if (o.port) name = name ? name + ':' + o.port : text(o.port);
      return name || text(o.ip);
    }

    function info(e) {
      if (e.method || e.url || e.path) {
        return [text(e.method), text(e.url || e.path), e.status_code ? '[' + e.status_code + ']' : ''].filter(Boolean).join(' ');
      }
      return text(e.summary);
    }

    function time(e) {
      var t = e.timestamp || e.index;
      if (typeof t === 'number') { var d = new Date(t < 1e12 ? t * 1000 : t); if (!isNaN(d)) return d.toLocaleTimeString(); }
      if (typeof t === 'string') { var d2 = new Date(t); if (!isNaN(d2)) return d2.toLocaleTimeString(); if (t) return t; }
      return '';
    }

    function add(e) {
      if (!e || typeof e !== 'object' || e.type === 'heartbeat') return;
      var tds = [time(e), endpoint(e.src), endpoint(e.dst), text(e.protocol), info(e) || JSON.stringify(e)];
      var tr = document.createElement('tr');
      tds.forEach(function (c) {
        var td = document.createElement('td');
        td.textContent = c;
        td.title = c;
        tr.appendChild(td);
      });
      empty.hidden = true;
      table.hidden = false;
      tbody.insertBefore(tr, tbody.firstChild);
      while (tbody.childNodes.length > MAX) tbody.removeChild(tbody.lastChild);
    }

    fetch('/api/metadata/version')
      .then(function (r) { return r.json(); })
      .then(function (v) {
        document.getElementById('version').textContent =
          'hub version ' + v.ver + (v.hubUrl ? ' (' + v.hubUrl + ')' : '');
      })
      .catch(function (e) {
        document.getElementById('version').textContent = 'hub unreachable: ' + e;
      });

    fetch('/api/flows2')
      .then(function (r) { return r.json(); })
      .then(function (list) { (Array.isArray(list) ? list : []).slice(-MAX).forEach(add); })
      .catch(function () {});

    try {
      var ws = new WebSocket((location.protocol === 'https:' ? 'wss:' : 'ws:') + '//' + location.host + '/ws');
      ws.onmessage = function (ev) {
        var e;
        try { e = JSON.parse(ev.data); } catch (_) { return; }
        add(e);
      };
    } catch (_) {}
  })();
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
