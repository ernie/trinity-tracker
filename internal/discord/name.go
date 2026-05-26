package discord

import "github.com/ernie/trinity-tracker/internal/domain"

// StripVRTag delegates to domain.StripVRTag.
func StripVRTag(name string) string { return domain.StripVRTag(name) }

// DisplayName returns name with the VR tag stripped only when isVR
// is true. A non-VR player who deliberately put "[VR]" in their
// chosen name keeps it — for those players the literal in the name
// is the only signal we have, since the platform badge would (and
// should) be absent.
func DisplayName(name string, isVR bool) string {
	if !isVR {
		return name
	}
	return domain.StripVRTag(name)
}
