package discord

import "github.com/ernie/trinity-tracker/internal/domain"

// StripVRTag delegates to domain.StripVRTag.
func StripVRTag(name string) string { return domain.StripVRTag(name) }

// DisplayName returns name with the VR tag stripped only when isVR
// is true. A flatscreen player who typed "[VR]" into their name
// keeps it, since no platform badge shows for them.
func DisplayName(name string, isVR bool) string {
	if !isVR {
		return name
	}
	return domain.StripVRTag(name)
}
