import {
  useState,
  useEffect,
  useCallback,
  createContext,
  useContext,
  createElement,
  type ReactNode,
} from "react";
import type { AuthState, LoginCredentials } from "../types";
import { AUTH_EXPIRED_EVENT } from "../authFetch";

interface AuthContextType {
  auth: AuthState;
  loading: boolean;
  login: (credentials: LoginCredentials) => Promise<boolean>;
  logout: () => Promise<void>;
  logoutEverywhere: () => Promise<void>;
  refresh: () => Promise<void>;
  changePassword: (
    currentPassword: string,
    newPassword: string,
  ) => Promise<{ success: boolean; error?: string; message?: string }>;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}

interface AuthProviderProps {
  children: ReactNode;
}

const loggedOut: AuthState = {
  isAuthenticated: false,
  username: null,
  isAdmin: false,
  playerId: null,
  passwordChangeRequired: false,
};

export function AuthProvider({ children }: AuthProviderProps) {
  const [auth, setAuth] = useState<AuthState>(loggedOut);
  const [loading, setLoading] = useState(true);

  // A promise chain, so every setState runs in a callback
  // (react-hooks/set-state-in-effect).
  const refresh = useCallback(
    (): Promise<void> =>
      fetch("/api/auth/check")
        .then((res) => res.json())
        .then((data) => {
          if (data.authenticated) {
            setAuth({
              isAuthenticated: true,
              username: data.username,
              isAdmin: data.is_admin || false,
              playerId: data.player_id || null,
              passwordChangeRequired: data.password_change_required || false,
            });
          } else {
            setAuth(loggedOut);
          }
        })
        .catch(() => {
          setAuth(loggedOut);
        })
        .finally(() => {
          setLoading(false);
        }),
    [],
  );

  useEffect(() => {
    refresh();
    const onExpired = () => {
      refresh();
    };
    window.addEventListener(AUTH_EXPIRED_EVENT, onExpired);
    return () => {
      window.removeEventListener(AUTH_EXPIRED_EVENT, onExpired);
    };
  }, [refresh]);

  const login = useCallback(
    async (credentials: LoginCredentials): Promise<boolean> => {
      try {
        const res = await fetch("/api/auth/login", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(credentials),
        });

        if (!res.ok) return false;

        const data = await res.json();
        setAuth({
          isAuthenticated: true,
          username: data.username,
          isAdmin: data.is_admin || false,
          playerId: data.player_id || null,
          passwordChangeRequired: data.password_change_required || false,
        });
        return true;
      } catch {
        return false;
      }
    },
    [],
  );

  // The cookie is HttpOnly, so only the server can expire it. Local state
  // clears whatever the request does.
  const logout = useCallback(async () => {
    try {
      await fetch("/api/auth/logout", { method: "POST" });
    } finally {
      setAuth(loggedOut);
    }
  }, []);

  // Bumps the account's token version, which ends its sessions on every device.
  const logoutEverywhere = useCallback(async () => {
    try {
      await fetch("/api/auth/logout-all", { method: "POST" });
    } finally {
      setAuth(loggedOut);
    }
  }, []);

  const changePassword = useCallback(
    async (
      currentPassword: string,
      newPassword: string,
    ): Promise<{ success: boolean; error?: string; message?: string }> => {
      try {
        const res = await fetch("/api/auth/change-password", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            current_password: currentPassword,
            new_password: newPassword,
          }),
        });

        const data = await res.json();
        if (!res.ok) {
          return {
            success: false,
            error: data.error || "Couldn't change your password.",
          };
        }

        // The server reissued this device's cookie; other devices are logged out.
        setAuth((prev) => ({ ...prev, passwordChangeRequired: false }));
        return { success: true, message: data.message };
      } catch {
        return { success: false, error: "Couldn't reach the server." };
      }
    },
    [],
  );

  const value = {
    auth,
    loading,
    login,
    logout,
    logoutEverywhere,
    refresh,
    changePassword,
  };

  return createElement(AuthContext.Provider, { value }, children);
}
