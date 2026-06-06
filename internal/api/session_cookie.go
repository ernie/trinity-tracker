package api

import (
	"net/http"
	"net/url"
)

// sessionCookieName carries the web-session JWT. HttpOnly keeps it out
// of reach of XSS; validity is governed by users.token_version, not the
// cookie's age.
const sessionCookieName = "trinity_session"

// sessionCookieMaxAge is the 400-day browser cap on cookie lifetime.
// The cookie is re-issued at every login.
const sessionCookieMaxAge = 34560000

// requestIsTLS reports whether the client connection is HTTPS. The
// hub itself serves plain HTTP; nginx terminates TLS and sets
// X-Forwarded-Proto. Plain-HTTP LAN installs get a non-Secure cookie.
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

// clearSessionCookie ends the device's session: HttpOnly means JS
// can't remove the cookie, so the server must expire it.
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

// cookieOriginAllowed is the CSRF gate for cookie-authenticated
// requests (cookies ride along automatically; Bearer headers never
// did). Non-GET requests bearing a cross-site Origin are refused.
// Absent Origin passes — non-browser clients don't send it, and
// SameSite=Lax is the backstop for browsers. Host-only compare: the
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
