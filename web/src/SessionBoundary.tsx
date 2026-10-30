import { useEffect, useState, useRef } from "react";
import App from "./App";
import {
  APIError,
  request,
  sessionExpiredEvent,
  setCSRFToken,
  type BrowserSession,
} from "./api";

export function SessionBoundary() {
  const generation = useRef(0);
  const panel = useRef<HTMLElement>(null);
  const [session, setSession] = useState<BrowserSession>();
  const [opened, setOpened] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function refresh() {
    const started = generation.current;
    try {
      const value = await request<BrowserSession>("/v1/auth/session");
      if (started !== generation.current) return;
      setCSRFToken(value.csrfToken ?? "");
      setSession(value);
      if (value.authenticated) setOpened(true);
      setError("");
    } catch (error) {
      if (started !== generation.current) return;
      if (error instanceof APIError && error.status === 401) {
        setCSRFToken("");
        setSession((previous) => ({
          ...previous,
          mode: "oidc",
          authenticated: false,
        }));
      } else
        setError(
          "Could not check your session. Retry when the server is available.",
        );
    }
  }
  useEffect(() => {
    void refresh();
    const expire = () => {
      generation.current++;
      setCSRFToken("");
      setSession((previous) => ({
        ...previous,
        mode: "oidc",
        authenticated: false,
      }));
    };
    window.addEventListener(sessionExpiredEvent, expire);
    const timer = setInterval(() => void refresh(), 30000);
    return () => {
      window.removeEventListener(sessionExpiredEvent, expire);
      clearInterval(timer);
      setCSRFToken("");
    };
  }, []);
  useEffect(() => {
    if (opened && session?.authenticated === false) {
      panel.current?.focus();
      panel.current?.scrollIntoView({ block: "start" });
    }
  }, [opened, session?.authenticated]);
  async function logout() {
    if (!window.confirm("Sign out? Unsaved changes will be lost.")) return;
    generation.current++;
    setBusy(true);
    try {
      await request("/v1/auth/logout", "POST");
      generation.current++;
      setCSRFToken("");
      setOpened(false);
      setSession((previous) => ({
        ...previous,
        mode: "oidc",
        authenticated: false,
      }));
    } catch {
      setError("Sign out did not complete. Retry to revoke this session.");
    } finally {
      setBusy(false);
    }
  }
  const failed =
    new URLSearchParams(window.location.search).get("login") === "failed";
  return (
    <>
      {(!session?.authenticated || error) && (
        <section
          ref={panel}
          tabIndex={-1}
          className="session-panel"
          aria-label="Account access"
        >
          <div className="eyebrow">RUNNELIX</div>
          <h1>
            {session?.authenticated
              ? "Session check unavailable"
              : opened
                ? "Your session has ended"
                : "Sign in to your workspace"}
          </h1>
          <p>
            {session
              ? opened
                ? "Sign in again to continue. Your current draft stays on this page until you leave."
                : "Use your organization’s sign-in provider to continue."
              : "Checking your session…"}
          </p>
          {(error || failed) && (
            <p role="alert" className="notice error">
              {error || "Sign-in could not be completed. Please try again."}
            </p>
          )}
          {session?.loginEnabled && (
            <a className="session-login" href="/v1/auth/login">
              Sign in
            </a>
          )}
          {session?.mode === "oidc" && session.loginEnabled === false && (
            <p>
              Browser sign-in is not configured. Contact your installation
              owner.
            </p>
          )}
          <button onClick={() => void refresh()}>Check session again</button>
        </section>
      )}
      {opened && (
        <div inert={!session?.authenticated}>
          <App
            sessionActions={
              session?.mode === "oidc" && (
                <button disabled={busy} onClick={() => void logout()}>
                  Sign out
                </button>
              )
            }
          />
        </div>
      )}
    </>
  );
}
