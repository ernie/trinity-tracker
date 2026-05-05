import { Link } from "react-router-dom";
import { AppLogo } from "./AppLogo";
import { PageNav } from "./PageNav";
import { LoginForm } from "./LoginForm";
import { MySourceButton } from "./MySourceButton";
import { DiscordButton } from "./DiscordButton";
import { useAuth } from "../hooks/useAuth";

interface HeaderProps {
  title: string;
  className?: string;
  linkToHome?: boolean;
  showDiscord?: boolean;
}

export function Header({ title, className, linkToHome, showDiscord }: HeaderProps) {
  const { auth, login, logout } = useAuth();

  return (
    <header className={className}>
      <h1>
        <AppLogo linkToHome={linkToHome} />
        {title}
        {showDiscord && <DiscordButton />}
      </h1>
      <PageNav />
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
    </header>
  );
}
