import { useState, useEffect, useRef, FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { ColoredText } from "./ColoredText";
import {
  DisplayNameEditor,
  useDisplayNameValidation,
} from "./DisplayNameEditor";
import { PlayerHero } from "./PlayerHero";
import { HonorsPanel } from "./HonorsPanel";
import { PortraitPickerModal } from "./PortraitPickerModal";
import { PlayerRecentMatches } from "./PlayerRecentMatches";
import { PeriodSelector } from "./PeriodSelector";
import { useAuth } from "../hooks/useAuth";
import { apiFetch } from "../authFetch";
import { usePlayerStats } from "../hooks/usePlayerStats";
import { formatDate, formatDateTime } from "../utils/formatters";
import { stripVRPrefix, canonicalizeDisplayName } from "../utils";
import type { AccountProfile, TimePeriod } from "../types";

export function AccountPage() {
  const navigate = useNavigate();
  const {
    auth,
    loading: authLoading,
    changePassword,
    logoutEverywhere,
  } = useAuth();

  const [profile, setProfile] = useState<AccountProfile | null>(null);
  const [period, setPeriod] = useState<TimePeriod>("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const {
    stats,
    refetch: refetchStats,
    setFeaturedHonor,
  } = usePlayerStats(profile?.player?.id, period);

  const [linkCode, setLinkCode] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<Date | null>(null);
  const [timeRemaining, setTimeRemaining] = useState(0);
  const [generatingCode, setGeneratingCode] = useState(false);
  const [linkError, setLinkError] = useState("");
  const timerRef = useRef<number | null>(null);

  const [claimCode, setClaimCode] = useState("");
  const [claimLoading, setClaimLoading] = useState(false);
  const [claimError, setClaimError] = useState("");
  const [claimSuccess, setClaimSuccess] = useState("");

  const [gameToken, setGameToken] = useState<string | null>(null);
  const [gameTokenLoading, setGameTokenLoading] = useState(false);
  const [gameTokenError, setGameTokenError] = useState("");
  const [gameTokenCopied, setGameTokenCopied] = useState(false);
  const [gameTokenVisible, setGameTokenVisible] = useState(false);
  const [showRotateConfirm, setShowRotateConfirm] = useState(false);

  const [editingDisplayName, setEditingDisplayName] = useState(false);
  const [displayNameValue, setDisplayNameValue] = useState("");
  const [displayNameError, setDisplayNameError] = useState("");
  const [savingDisplayName, setSavingDisplayName] = useState(false);

  const [showPortraitPicker, setShowPortraitPicker] = useState(false);

  const [showPasswordForm, setShowPasswordForm] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const [changingPassword, setChangingPassword] = useState(false);
  const [passwordSuccess, setPasswordSuccess] = useState("");

  const displayNameValidation = useDisplayNameValidation(
    displayNameValue,
    "restyle",
    profile?.player
      ? canonicalizeDisplayName(
          profile.player.is_vr
            ? stripVRPrefix(profile.player.name)
            : profile.player.name,
        )
      : undefined,
  );

  const handleSaveDisplayName = async () => {
    setSavingDisplayName(true);
    setDisplayNameError("");
    try {
      const res = await apiFetch("/api/account/display-name", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ display_name: displayNameValidation.cleaned }),
      });
      if (!res.ok) {
        const data = await res.json();
        setDisplayNameError(data.error || "Couldn't update your display name.");
        return;
      }
      setEditingDisplayName(false);
      const profileRes = await apiFetch("/api/account/profile");
      if (profileRes.ok) {
        setProfile(await profileRes.json());
      }
      refetchStats();
    } catch {
      setDisplayNameError("Couldn't reach the server.");
    } finally {
      setSavingDisplayName(false);
    }
  };

  // The backend coalesces the portrait into player.model, so the hero
  // only updates after a refetch.
  const handlePortraitSaved = async () => {
    setShowPortraitPicker(false);
    const profileRes = await apiFetch("/api/account/profile");
    if (profileRes.ok) {
      setProfile(await profileRes.json());
    }
    refetchStats();
  };

  useEffect(() => {
    if (!authLoading && !auth.isAuthenticated) {
      navigate("/");
    }
  }, [authLoading, auth.isAuthenticated, navigate]);

  useEffect(() => {
    if (!auth.isAuthenticated) return;

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    apiFetch("/api/account/profile")
      .then((res) => {
        if (!res.ok) throw new Error("Couldn't load your profile.");
        return res.json();
      })
      .then((data) => setProfile(data))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [auth.isAuthenticated]);

  useEffect(() => {
    if (!expiresAt) return;

    const updateTimer = () => {
      const now = new Date();
      const remaining = Math.max(
        0,
        Math.floor((expiresAt.getTime() - now.getTime()) / 1000),
      );
      setTimeRemaining(remaining);

      if (remaining === 0) {
        setLinkCode(null);
        setExpiresAt(null);
      }
    };

    updateTimer();
    timerRef.current = window.setInterval(updateTimer, 1000);

    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
      }
    };
  }, [expiresAt]);

  useEffect(() => {
    if (!auth.isAuthenticated) return;

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setGameTokenLoading(true);
    apiFetch("/api/auth/game-token")
      .then((res) => {
        if (!res.ok) throw new Error("Couldn't load your game token.");
        return res.json();
      })
      .then((data) => setGameToken(data.token))
      .catch((err) => setGameTokenError(err.message))
      .finally(() => setGameTokenLoading(false));
  }, [auth.isAuthenticated]);

  const handleCopyToken = async () => {
    if (!gameToken) return;
    try {
      await navigator.clipboard.writeText(gameToken);
      setGameTokenCopied(true);
      setTimeout(() => setGameTokenCopied(false), 2000);
    } catch {
      setGameTokenError("Couldn't copy to the clipboard.");
    }
  };

  const handleRotateToken = async () => {
    setGameTokenLoading(true);
    setGameTokenError("");
    setShowRotateConfirm(false);

    try {
      const res = await apiFetch("/api/auth/game-token", {
        method: "POST",
      });

      if (!res.ok) {
        const data = await res.json();
        setGameTokenError(data.error || "Couldn't rotate your token.");
        return;
      }

      const data = await res.json();
      setGameToken(data.token);
    } catch {
      setGameTokenError("Couldn't reach the server.");
    } finally {
      setGameTokenLoading(false);
    }
  };

  const handleClaimLink = async () => {
    if (claimCode.length !== 6) return;
    setClaimLoading(true);
    setClaimError("");
    setClaimSuccess("");

    try {
      const validateRes = await fetch("/api/claim/validate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code: claimCode }),
      });
      if (!validateRes.ok) {
        const data = await validateRes.json();
        setClaimError(data.error || "That code is wrong or has expired.");
        return;
      }

      const res = await apiFetch("/api/claim/link", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code: claimCode }),
      });
      const data = await res.json();
      if (!res.ok) {
        setClaimError(data.error || "Couldn't link this player.");
        return;
      }
      setClaimSuccess(`${data.message} Refresh the page to see it.`);
      setClaimCode("");
    } catch {
      setClaimError("Couldn't reach the server.");
    } finally {
      setClaimLoading(false);
    }
  };

  const generateCode = async () => {
    setGeneratingCode(true);
    setLinkError("");

    try {
      const res = await apiFetch("/api/account/link-code", {
        method: "POST",
      });

      if (!res.ok) {
        const data = await res.json();
        setLinkError(data.error || "Couldn't generate a code.");
        return;
      }

      const data = await res.json();
      setLinkCode(data.code);
      setExpiresAt(new Date(data.expires_at));
    } catch {
      setLinkError("Couldn't reach the server.");
    } finally {
      setGeneratingCode(false);
    }
  };

  const formatTime = (seconds: number): string => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${secs.toString().padStart(2, "0")}`;
  };

  const handlePasswordSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setPasswordError("");
    setPasswordSuccess("");

    if (newPassword.length < 8) {
      setPasswordError("Password must be at least 8 characters.");
      return;
    }

    if (newPassword !== confirmPassword) {
      setPasswordError("The passwords don't match.");
      return;
    }

    setChangingPassword(true);
    const result = await changePassword(currentPassword, newPassword);
    setChangingPassword(false);

    if (!result.success) {
      setPasswordError(result.error || "Couldn't change your password.");
    } else {
      setPasswordSuccess(result.message ?? "");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setShowPasswordForm(false);
    }
  };

  if (authLoading || !auth.isAuthenticated) {
    return (
      <div className="account-page">
        <div className="loading">Loading…</div>
      </div>
    );
  }

  return (
    <div className="account-page">
      {loading && <div className="loading">Loading…</div>}
      {error && <div className="error-message">{error}</div>}

      {profile && (
        <div className="account-columns">
          <div className="account-left">
            <section className="account-section account-profile">
              <h2>Player profile</h2>
              {profile.player ? (
                <>
                  {/* Same stack as PlayersPage and PlayerStatsModal; keep
                      the three in step. */}
                  {stats ? (
                    <>
                      <PlayerHero
                        player={stats.player}
                        stats={stats.stats}
                        variant="page"
                        onPortraitClick={() => setShowPortraitPicker(true)}
                      />
                      {showPortraitPicker && (
                        <PortraitPickerModal
                          current={profile.user.portrait ?? null}
                          onSaved={() => {
                            void handlePortraitSaved();
                          }}
                          onClose={() => setShowPortraitPicker(false)}
                        />
                      )}
                      <PeriodSelector period={period} onChange={setPeriod} />
                      <HonorsPanel
                        stats={stats.stats}
                        featuredKey={stats.player.featured_honor}
                        isOwner
                        onFeaturedChange={(key) => {
                          void setFeaturedHonor(key);
                        }}
                      />
                    </>
                  ) : (
                    <PeriodSelector period={period} onChange={setPeriod} />
                  )}
                </>
              ) : (
                <p className="no-player">
                  No player profile linked to this account.
                </p>
              )}
            </section>

            {profile.guids && profile.guids.length > 0 && (
              <section className="account-section account-guids">
                <h2>Your game identities ({profile.guids.length})</h2>
                <div className="guids-list">
                  {profile.guids.map((guid) => (
                    <div key={guid.id} className="guid-item">
                      <div className="guid-main">
                        <ColoredText text={guid.name} />
                        <span className="guid-hash" title={guid.guid}>
                          {guid.guid.slice(0, 8)}…
                        </span>
                      </div>
                      <div className="guid-dates">
                        {formatDate(guid.first_seen)} –{" "}
                        {formatDate(guid.last_seen)}
                      </div>
                    </div>
                  ))}
                </div>
              </section>
            )}

            {profile.player && (
              <section className="account-section account-matches">
                <PlayerRecentMatches playerId={profile.player.id} />
              </section>
            )}
          </div>

          <div className="account-right">
            <section className="account-section account-info">
              <h2>Account information</h2>
              <dl className="info-list">
                <dt>Username</dt>
                <dd>{profile.user.username}</dd>
                <dt>Display name</dt>
                <dd>
                  {editingDisplayName ? (
                    <div className="display-name-edit-inline">
                      <DisplayNameEditor
                        value={displayNameValue}
                        onChange={setDisplayNameValue}
                        mode="restyle"
                        originalCanonical={
                          profile.player
                            ? canonicalizeDisplayName(
                                profile.player.is_vr
                                  ? stripVRPrefix(profile.player.name)
                                  : profile.player.name,
                              )
                            : ""
                        }
                        error={displayNameError}
                        onSubmit={
                          !displayNameValidation.canonicalMismatch &&
                          !displayNameValidation.isEmpty
                            ? handleSaveDisplayName
                            : undefined
                        }
                      />
                      <div className="display-name-edit-actions">
                        <button
                          className="claim-primary-btn"
                          disabled={
                            savingDisplayName ||
                            displayNameValidation.canonicalMismatch ||
                            displayNameValidation.isEmpty
                          }
                          onClick={handleSaveDisplayName}
                        >
                          {savingDisplayName ? "Saving…" : "Save"}
                        </button>
                        <button
                          className="claim-secondary-btn"
                          onClick={() => {
                            setEditingDisplayName(false);
                            setDisplayNameError("");
                          }}
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  ) : (
                    <span className="display-name-static">
                      {profile.player ? (
                        <ColoredText text={profile.player.name} />
                      ) : (
                        <span className="muted">Not set</span>
                      )}
                      {profile.player && (
                        <button
                          className="edit-icon-btn"
                          onClick={() => {
                            setDisplayNameValue(profile.player!.name);
                            setEditingDisplayName(true);
                            setDisplayNameError("");
                          }}
                          title="Edit display name colors"
                        >
                          &#9998;
                        </button>
                      )}
                    </span>
                  )}
                </dd>
                <dt>Role</dt>
                <dd>{profile.user.is_admin ? "Administrator" : "User"}</dd>
                <dt>Account created</dt>
                <dd>{formatDateTime(profile.user.created_at)}</dd>
                <dt>Last login</dt>
                <dd>
                  {profile.user.last_login
                    ? formatDateTime(profile.user.last_login)
                    : "First login"}
                </dd>
              </dl>
            </section>

            <section className="account-section account-password">
              <h2>Security</h2>
              {passwordSuccess && (
                <div className="success-message">{passwordSuccess}</div>
              )}
              {showPasswordForm ? (
                <form onSubmit={handlePasswordSubmit} className="password-form">
                  <div className="form-group">
                    <label>Current password</label>
                    <input
                      type="password"
                      value={currentPassword}
                      onChange={(e) => setCurrentPassword(e.target.value)}
                      disabled={changingPassword}
                      autoComplete="current-password"
                    />
                  </div>
                  <div className="form-group">
                    <label>New password</label>
                    <input
                      type="password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      disabled={changingPassword}
                      autoComplete="new-password"
                    />
                  </div>
                  <div className="form-group">
                    <label>Confirm new password</label>
                    <input
                      type="password"
                      value={confirmPassword}
                      onChange={(e) => setConfirmPassword(e.target.value)}
                      disabled={changingPassword}
                      autoComplete="new-password"
                    />
                  </div>
                  {passwordError && (
                    <div className="error-message">{passwordError}</div>
                  )}
                  <div className="form-actions">
                    <button
                      type="submit"
                      disabled={
                        changingPassword ||
                        !currentPassword ||
                        !newPassword ||
                        !confirmPassword
                      }
                    >
                      {changingPassword ? "Changing…" : "Change password"}
                    </button>
                    <button
                      type="button"
                      className="cancel-btn"
                      onClick={() => {
                        setShowPasswordForm(false);
                        setCurrentPassword("");
                        setNewPassword("");
                        setConfirmPassword("");
                        setPasswordError("");
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              ) : (
                <button
                  onClick={() => setShowPasswordForm(true)}
                  className="change-password-btn"
                >
                  Change password
                </button>
              )}
              <div className="logout-everywhere">
                <p className="link-explanation-text">
                  Signs you out of every browser and device. Use it if a session
                  may have leaked.
                </p>
                <button
                  onClick={() => {
                    void logoutEverywhere().then(() => navigate("/"));
                  }}
                >
                  Log out everywhere
                </button>
              </div>
            </section>

            <section className="account-section account-game-token">
              <h2>Game token</h2>
              <p className="link-explanation-text">
                Set <code>cl_trinityUser</code> to your username and{" "}
                <code>cl_trinityToken</code> to this token in your{" "}
                <code>autoexec.cfg</code>, or log in from{" "}
                <strong>Account</strong> on the game&rsquo;s main menu.
              </p>

              {gameTokenError && (
                <div className="error-message">{gameTokenError}</div>
              )}

              {gameTokenLoading && !gameToken ? (
                <div className="loading">Loading…</div>
              ) : gameToken ? (
                <div className="game-token-display">
                  <div
                    className={`game-token-value${gameTokenVisible ? "" : " hidden"}`}
                  >
                    {gameToken}
                  </div>
                  <div className="game-token-actions">
                    <button
                      className="generate-btn rotate-btn"
                      onClick={() => setGameTokenVisible(!gameTokenVisible)}
                    >
                      {gameTokenVisible ? "Hide" : "Reveal"}
                    </button>
                    <button
                      className="generate-btn rotate-btn"
                      onClick={handleCopyToken}
                    >
                      {gameTokenCopied ? "Copied!" : "Copy"}
                    </button>
                    {showRotateConfirm ? (
                      <div className="rotate-confirm">
                        <span className="rotate-confirm-text">
                          Replace this token? Installs using it will need the
                          new one.
                        </span>
                        <div className="rotate-confirm-actions">
                          <button
                            className="generate-btn"
                            onClick={handleRotateToken}
                            disabled={gameTokenLoading}
                          >
                            {gameTokenLoading ? "Rotating…" : "Confirm"}
                          </button>
                          <button
                            className="cancel-btn"
                            onClick={() => setShowRotateConfirm(false)}
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    ) : (
                      <button
                        className="generate-btn"
                        onClick={() => setShowRotateConfirm(true)}
                      >
                        Rotate
                      </button>
                    )}
                  </div>
                </div>
              ) : null}
            </section>

            <section className="account-section account-link">
              <h2>Link a game identity</h2>

              <div className="link-method">
                <h3>From the game</h3>
                <p className="link-explanation-text">
                  Type <code>!claim</code> in chat on a Trinity server to get a
                  code, then enter it here.
                </p>
                <div className="claim-code-inline">
                  <input
                    type="text"
                    value={claimCode}
                    onChange={(e) => {
                      const val = e.target.value.replace(/\D/g, "").slice(0, 6);
                      setClaimCode(val);
                      setClaimError("");
                      setClaimSuccess("");
                    }}
                    placeholder="000000"
                    maxLength={6}
                    className="claim-code-field"
                  />
                  <button
                    onClick={handleClaimLink}
                    disabled={claimCode.length !== 6 || claimLoading}
                    className="generate-btn"
                  >
                    {claimLoading ? "Linking…" : "Link"}
                  </button>
                </div>
                {claimError && (
                  <div className="error-message">{claimError}</div>
                )}
                {claimSuccess && (
                  <div className="success-message">{claimSuccess}</div>
                )}
              </div>

              {profile.player && (
                <div className="link-method">
                  <h3>From the web</h3>
                  <p className="link-explanation-text">
                    Generate a code here, then type{" "}
                    <code>!link &lt;code&gt;</code> in chat on the install you
                    want to link.
                  </p>

                  {linkError && (
                    <div className="error-message">{linkError}</div>
                  )}

                  {linkCode ? (
                    <div className="code-display">
                      <div className="code-label">Your link code</div>
                      <div className="code-value">{linkCode}</div>
                      <div className="code-timer">
                        Expires in <strong>{formatTime(timeRemaining)}</strong>
                      </div>
                      <div className="code-instruction">
                        Type in chat: <code>!link {linkCode}</code>
                      </div>
                      <button
                        onClick={generateCode}
                        disabled={generatingCode}
                        className="generate-btn"
                      >
                        {generatingCode ? "Generating…" : "Generate new code"}
                      </button>
                    </div>
                  ) : (
                    <button
                      className="generate-btn"
                      onClick={generateCode}
                      disabled={generatingCode}
                    >
                      {generatingCode ? "Generating…" : "Generate link code"}
                    </button>
                  )}
                </div>
              )}
            </section>
          </div>
        </div>
      )}
    </div>
  );
}
