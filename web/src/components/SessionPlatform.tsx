import { platformName } from "../utils/platform";

interface SessionPlatformProps {
  platform?: string;
  isVR: boolean;
}

export function SessionPlatform({ platform, isVR }: SessionPlatformProps) {
  return (
    <span className="session-platform">
      {platform ? platformName(platform) : "—"}
      {isVR && (
        <img
          className="session-platform__vr"
          src="/assets/vr/vr.png"
          alt="VR"
          title="Played in VR"
        />
      )}
    </span>
  );
}
