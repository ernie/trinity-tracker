// A 401 means the session ended mid-use (revoked from another device, or the
// account was disabled); AuthProvider hears the event and rechecks auth.

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
