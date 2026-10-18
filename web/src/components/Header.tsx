import { Link } from "react-router-dom";
import { AppLogo } from "./AppLogo";
import { PageNav } from "./PageNav";
import { NavScroller } from "./NavScroller";
import { LoginForm } from "./LoginForm";
import { MySourceButton } from "./MySourceButton";
import { CommunityCluster } from "./CommunityCluster";
import { HamburgerMenu } from "./HamburgerMenu";
import { useAuth } from "../hooks/useAuth";
import { useLiveData } from "../contexts/LiveDataContext";

interface HeaderProps {
  title: string;
  className?: string;
  linkToHome?: boolean;
  // Subtitle beside the title; the home page uses it to tell the tracker
  // apart from Trinity itself.
  wordmark?: string;
  // Sits over the landing hero with no background or border.
  transparent?: boolean;
  // With `transparent`, restores the solid look once the page scrolls
  // past the hero.
  solid?: boolean;
}

export function Header({
  title,
  className,
  linkToHome,
  wordmark,
  transparent,
  solid,
}: HeaderProps) {
  const { auth, login, logout } = useAuth();
  const { setCommandPaletteOpen } = useLiveData();
  // iOS uses ⌘ with an external keyboard, and iPhones and older iPads
  // don't report "Mac".
  const isApple =
    typeof navigator !== "undefined" &&
    /mac|iphone|ipad|ipod/i.test(navigator.platform);

  return (
    <header
      className={`${className ?? ""}${transparent ? " app-header--transparent" : ""}${solid ? " app-header--solid" : ""}`}
    >
      <div className="app-header__brand">
        <AppLogo linkToHome={linkToHome} />
        <h1>
          {title}
          {wordmark && <span className="app-wordmark">{wordmark}</span>}
        </h1>
        <div className="app-header__search">
          <button
            type="button"
            className="cmdk-trigger cmdk-trigger--full"
            onClick={() => setCommandPaletteOpen(true)}
            aria-label="Open command palette"
          >
            <span className="cmdk-trigger__icon" aria-hidden="true">
              <svg
                width="14"
                height="14"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <circle cx="7" cy="7" r="4.5" />
                <path d="M14 14L10.5 10.5" />
              </svg>
            </span>
            <span className="cmdk-trigger__placeholder">Search</span>
            <kbd className="cmdk-trigger__kbd">{isApple ? "⌘K" : "Ctrl K"}</kbd>
          </button>
        </div>
      </div>

      <div className="app-header__row">
        <NavScroller>
          <PageNav />
        </NavScroller>
        <HamburgerMenu
          className={
            auth.isAuthenticated
              ? `hamburger--authed${auth.isAdmin ? " hamburger--admin" : ""}`
              : ""
          }
        >
          <CommunityCluster />
          <div className="auth-section">
            {auth.isAuthenticated ? (
              <div className="user-info">
                <MySourceButton />
                {auth.isAdmin && (
                  <Link to="/admin" className="admin-btn">
                    Admin
                  </Link>
                )}
                <Link to="/account" className="username-link">
                  {auth.username}
                </Link>
                <button onClick={logout} className="logout-btn">
                  Log out
                </button>
              </div>
            ) : (
              <LoginForm
                onLogin={(username, password) => login({ username, password })}
              />
            )}
          </div>
        </HamburgerMenu>
      </div>
    </header>
  );
}
