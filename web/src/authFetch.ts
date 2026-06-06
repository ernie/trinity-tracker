// fetch wrapper for authenticated API calls. The session cookie rides
// along automatically (same-origin default); a 401 means the session
// died mid-use (token-version bump from another device, disable), so
// broadcast it — AuthProvider listens and re-derives auth state.

export const AUTH_EXPIRED_EVENT = "auth-expired";

export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  const res = await fetch(input, init);
  if (res.status === 401) {
    window.dispatchEvent(new Event(AUTH_EXPIRED_EVENT));
  }
  return res;
}
