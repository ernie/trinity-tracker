package api

import (
	"net/http"
	"strconv"
)

var validPeriods = map[string]bool{
	"all": true, "day": true, "week": true, "month": true, "year": true,
}

var validGameTypes = map[string]bool{
	"ffa": true, "tdm": true, "ctf": true, "1fctf": true,
	"1v1": true, "overload": true, "harvester": true,
}

// Modes are g_mode values: 0 Quake III Arena, 1 CPMA, 2 Quake Live,
// 3 Quake Live Turbo.
var validModes = map[string]bool{"0": true, "1": true, "2": true, "3": true}

var validCategories = map[string]bool{
	"frags": true, "deaths": true, "kd_ratio": true, "matches": true,
	"captures": true, "flag_returns": true, "assists": true,
	"impressives": true, "excellents": true, "humiliations": true,
	"defends": true, "victories": true,
	"skulls_delivered": true, "obelisks_destroyed": true,
}

// parseLimit returns the limit parameter, or defaultLimit when it is
// missing, not positive or above maxLimit.
func parseLimit(r *http.Request, defaultLimit, maxLimit int) int {
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxLimit {
			return parsed
		}
	}
	return defaultLimit
}

func parseOffset(r *http.Request) int {
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return 0
}

// parseBeforeID returns the "before" pagination cursor, or nil.
func parseBeforeID(r *http.Request) *int64 {
	if b := r.URL.Query().Get("before"); b != "" {
		if parsed, err := strconv.ParseInt(b, 10, 64); err == nil && parsed > 0 {
			return &parsed
		}
	}
	return nil
}

func validatePeriod(period string) bool {
	return validPeriods[period]
}

func validateGameType(gameType string) bool {
	return validGameTypes[gameType]
}

func validateCategory(category string) bool {
	return validCategories[category]
}

func validateMode(m string) bool { return validModes[m] }
