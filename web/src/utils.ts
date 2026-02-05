// Format a number with commas (e.g., 1234567 -> "1,234,567")
export function formatNumber(n: number): string {
  return n.toLocaleString();
}

// Strip [VR] prefix from player name when VR badge is shown
// Handles color codes around [VR]: e.g., "^7[VR]^1Name" or "[VR] Name"
export function stripVRPrefix(name: string): string {
  return name.replace(/^(\^[0-9])*\[VR\]\s*/i, '');
}
