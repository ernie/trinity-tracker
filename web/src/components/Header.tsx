import { Link } from "react-router-dom";
import { AppLogo } from "./AppLogo";
import { PageNav } from "./PageNav";
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
  // Small subtitle next to the page title in the brand row. Used on the
  // home page (title="Trinity") to disambiguate the tracker site from
  // the Trinity game/community itself.
  wordmark?: string;
  // When true, the header sits over the LandingPage hero with no solid
  // background or border. Task 26 toggles `app-header--solid` on scroll
  // past the hero to flip it back to its standard look.
  transparent?: boolean;
  // Pairs with `transparent`: when both are true, the header renders solid
  // (post-scroll). Used by LandingPage which observes a sentinel in the hero.
  solid?: boolean;
}

export function Header({ title, className, linkToHome, wordmark, transparent, solid }: HeaderProps) {
  const { auth, login, logout } = useAuth();
  const { setCommandPaletteOpen } = useLiveData();
  // All Apple platforms (Mac, iPhone, iPad, iPod) use ⌘ for system
  // shortcuts — including iOS with an attached external keyboard. The
  // earlier /mac/i check missed iPhone/iPad and showed "Ctrl K" there,
  // which reads as alien on iOS. Modern iPadOS reports "MacIntel" so
  // it already matches /mac/, but iPhone and older iPad don't.
  const isApple = typeof navigator !== 'undefined' &&
    /mac|iphone|ipad|ipod/i.test(navigator.platform);

  return (
    <header className={`${className ?? ''}${transparent ? ' app-header--transparent' : ''}${solid ? ' app-header--solid' : ''}`}>
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
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="7" cy="7" r="4.5" />
                <path d="M14 14L10.5 10.5" />
              </svg>
            </span>
            <span className="cmdk-trigger__placeholder">Search</span>
            <kbd className="cmdk-trigger__kbd">{isApple ? '⌘K' : 'Ctrl K'}</kbd>
          </button>
        </div>
      </div>

      <div className="app-header__row">
        <div className="app-header__nav-scroll">
          <PageNav />
        </div>
        <HamburgerMenu>
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
                  Logout
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
