import { useLocation } from "react-router-dom";
import { Header } from "./Header";

// Mounted once at the shell so the header survives navigation. Routes
// that draw their own header get null.
function deriveTitle(
  pathname: string,
): { title: string; wordmark?: string } | null {
  if (pathname === "/") return null;
  if (pathname === "/play") return null;
  if (/^\/matches\/\d+\/demo$/.test(pathname)) return null;
  if (pathname.startsWith("/tv/")) return null;

  if (/^\/matches\/\d+/.test(pathname)) return { title: "Match details" };
  if (pathname.startsWith("/matches")) return { title: "Matches" };
  if (pathname.startsWith("/players")) return { title: "Players" };
  if (pathname.startsWith("/leaderboard")) return { title: "Leaderboard" };
  if (pathname.startsWith("/docs")) return { title: "Docs" };
  if (pathname.startsWith("/account")) return { title: "My account" };
  if (pathname.startsWith("/admin")) return { title: "Admin" };
  if (pathname.startsWith("/claim")) return { title: "Claim identity" };
  if (pathname.startsWith("/quake3-eula"))
    return { title: "Quake III Arena EULA" };
  return { title: "Trinity", wordmark: "tracker" };
}

export function RouteHeader() {
  const { pathname } = useLocation();
  const meta = deriveTitle(pathname);
  if (!meta) return null;
  return (
    <Header
      title={meta.title}
      className="app-header"
      wordmark={meta.wordmark}
    />
  );
}
