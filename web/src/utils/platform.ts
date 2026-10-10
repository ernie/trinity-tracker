const PLATFORM_NAMES: Record<string, string> = {
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
  frame: "Steam Frame",
  quest: "Quest",
  pico: "PICO",
  unknown: "Unknown",
};

// older sessions have no platform
export function platformName(platform?: string): string {
  if (!platform) return "Not recorded";
  return PLATFORM_NAMES[platform] ?? platform;
}
