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

var validCategories = map[string]bool{
	"kills": true, "deaths": true, "kd_ratio": true, "matches": true,
	"captures": true, "flag_returns": true, "assists": true,
	"impressives": true, "excellents": true, "humiliations": true,
	"defends": true, "victories": true,
}

// parseLimit parses and validates a limit parameter with default and max values
func parseLimit(r *http.Request, defaultLimit, maxLimit int) int {
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxLimit {
			return parsed
		}
	}
	return defaultLimit
}

// parseOffset parses and validates an offset parameter
func parseOffset(r *http.Request) int {
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return 0
}

// parseBeforeID parses and validates a cursor-based pagination parameter
func parseBeforeID(r *http.Request) *int64 {
	if b := r.URL.Query().Get("before"); b != "" {
		if parsed, err := strconv.ParseInt(b, 10, 64); err == nil && parsed > 0 {
			return &parsed
		}
	}
	return nil
}

// validatePeriod checks if a period string is valid
func validatePeriod(period string) bool {
	return validPeriods[period]
}

// validateGameType checks if a game type string is valid
func validateGameType(gameType string) bool {
	return validGameTypes[gameType]
}

// validateCategory checks if a leaderboard category is valid
func validateCategory(category string) bool {
	return validCategories[category]
}
