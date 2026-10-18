package api

import (
	"net/http"
	"net/url"
)

// sessionCookieName carries the session JWT, HttpOnly so scripts can't
// read it. users.token_version decides whether it is still valid.
const sessionCookieName = "trinity_session"

// sessionCookieMaxAge is the browsers' 400-day cap on cookie lifetime.
const sessionCookieMaxAge = 34560000

// requestIsTLS reports whether the client connected over HTTPS. nginx
// terminates TLS and sets X-Forwarded-Proto; a plain-HTTP LAN install
// gets a cookie without Secure.
func requestIsTLS(req *http.Request) bool {
	return req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https"
}

func setSessionCookie(w http.ResponseWriter, req *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   sessionCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsTLS(req),
	})
}

// clearSessionCookie expires the cookie, which scripts can't remove
// because it is HttpOnly.
func clearSessionCookie(w http.ResponseWriter, req *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsTLS(req),
	})
}

// cookieOriginAllowed is the CSRF check for cookie-authenticated
// requests: a write with a cross-site Origin is refused. A request with
// no Origin passes, since non-browser clients don't send one and
// SameSite=Lax covers browsers. Only the host is compared, because the
// hub doesn't know its public scheme.
func cookieOriginAllowed(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead:
		return true
	}
	origin := req.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == req.Host
}
