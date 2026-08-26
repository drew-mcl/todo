package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// A server on 127.0.0.1 is not a private server. Any page you visit can script
// a request to it, and because a POST with a CORS-safelisted content type is
// never preflighted, the browser will send it and this process will act on it.
// Reading the response needs DNS rebinding -- a hostname the attacker controls
// resolving to 127.0.0.1, which makes their page same-origin with this one.
//
// Both are closed by refusing to answer unless the request was addressed to
// loopback and, if it names an origin, that origin is us.

const maxBody = 4 << 20 // 4 MiB is a very large paste and a very small DoS

func isLoopbackHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// guard wraps the API. Static assets go through it too: the client is only ever
// served to the same browser that is allowed to call the API.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Defeats DNS rebinding: the Host the browser was told to connect to
		// must itself be loopback, not an attacker's domain pointed at us.
		if !isLoopbackHost(r.Host) {
			http.Error(w, "This server only answers on localhost.", http.StatusForbidden)
			return
		}

		// A browser sends Origin on every cross-site request and on all POSTs.
		// Its absence means a same-origin GET or a non-browser client.
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLoopbackHost(u.Host) || u.Host != r.Host {
				w.Header().Set("Vary", "Origin")
				http.Error(w, "Refused: that request came from another site.", http.StatusForbidden)
				return
			}
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}
