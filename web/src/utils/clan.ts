// Keys match the Go ClanKey that names the extracted logo files.
export function clanLogoUrl(
  name: string | undefined,
  team: "red" | "blue",
): string | undefined {
  const key = (name ?? "").replace(/\s+/g, "").toLowerCase();
  if (!key || key === "red" || key === "blue") return undefined;
  return `/assets/clans/${encodeURIComponent(key)}_${team}.png`;
}
