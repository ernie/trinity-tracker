package discord

import "regexp"

// vrLeadingRe matches a leading "[VR]" tag, optionally preceded by
// Q3 color runs (e.g. "^7[VR] alice") and followed by whitespace.
// The frontend uses an equivalent regex in web/src/utils.ts; keep
// them in sync.
var vrLeadingRe = regexp.MustCompile(`(?i)^(?:\^[0-9])*\[VR\]\s*`)

// vrTrailingRe matches a trailing "[VR]" tag with optional leading
// whitespace and trailing color runs ("alice [VR]^7").
var vrTrailingRe = regexp.MustCompile(`(?i)\s*\[VR\](?:\^[0-9])*$`)

// StripVRTag removes the "[VR]" decoration the Trinity VR client
// adds to a player's name, at either the start or end. Returns the
// name unchanged if no tag is present. Prefer DisplayName when an
// IsVR flag is available — see its docstring for why.
func StripVRTag(name string) string {
	if loc := vrLeadingRe.FindStringIndex(name); loc != nil {
		return name[loc[1]:]
	}
	if loc := vrTrailingRe.FindStringIndex(name); loc != nil {
		return name[:loc[0]]
	}
	return name
}

// DisplayName returns name with the VR tag stripped only when isVR
// is true. A non-VR player who deliberately put "[VR]" in their
// chosen name keeps it — for those players the literal in the name
// is the only signal we have, since the platform badge would (and
// should) be absent.
func DisplayName(name string, isVR bool) string {
	if !isVR {
		return name
	}
	return StripVRTag(name)
}
