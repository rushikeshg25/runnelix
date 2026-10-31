import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import {
  applyTheme,
  readTheme,
  resolveTheme,
  themeStorageKey,
  type ThemePreference,
} from "./theme";

const ThemeContext = createContext({
  resolved: "light" as "light" | "dark",
  setPreference: (_value: ThemePreference) => {},
});

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreference] = useState(readTheme);
  const [systemDark, setSystemDark] = useState(
    () => matchMedia("(prefers-color-scheme: dark)").matches,
  );
  useEffect(() => {
    const media = matchMedia("(prefers-color-scheme: dark)");
    const update = () => setSystemDark(media.matches);
    media.addEventListener("change", update);
    update();
    const sync = (event: StorageEvent) => {
      if (event.key === themeStorageKey || event.key === null)
        setPreference(readTheme());
    };
    window.addEventListener("storage", sync);
    return () => {
      media.removeEventListener("change", update);
      window.removeEventListener("storage", sync);
    };
  }, []);
  useEffect(() => {
    applyTheme(preference, systemDark);
  }, [preference, systemDark]);
  function choose(value: ThemePreference) {
    setPreference(value);
    try {
      localStorage.setItem(themeStorageKey, value);
    } catch {
      // A blocked store must not prevent changing the current theme.
    }
  }
  return (
    <ThemeContext.Provider
      value={{
        resolved: resolveTheme(preference, systemDark),
        setPreference: choose,
      }}
    >
      {children}
    </ThemeContext.Provider>
  );
}

export const useTheme = () => useContext(ThemeContext);

export function ThemePicker() {
  const { resolved, setPreference } = useTheme();
  const dark = resolved === "dark";
  const label = dark ? "Switch to light mode" : "Switch to dark mode";
  return (
    <button
      type="button"
      className="theme-toggle"
      aria-label={label}
      title={label}
      onClick={() => setPreference(dark ? "light" : "dark")}
    >
      <svg
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.7"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        {dark ? (
          <>
            <circle cx="12" cy="12" r="4" />
            <path d="M12 2v2m0 16v2M2 12h2m16 0h2M4.93 4.93l1.42 1.42m11.3 11.3 1.42 1.42M4.93 19.07l1.42-1.42m11.3-11.3 1.42-1.42" />
          </>
        ) : (
          <path d="M20.9 13.2A9 9 0 0 1 10.8 3.1a9 9 0 1 0 10.1 10.1Z" />
        )}
      </svg>
    </button>
  );
}
