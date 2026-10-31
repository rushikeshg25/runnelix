import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import {
  applyTheme,
  parseTheme,
  readTheme,
  resolveTheme,
  themeStorageKey,
  type ThemePreference,
} from "./theme";

const ThemeContext = createContext({
  preference: "system" as ThemePreference,
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
        preference,
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
  const { preference, setPreference } = useTheme();
  return (
    <label className="theme-control">
      <span>Theme</span>
      <select
        aria-label="Color theme"
        value={preference}
        onChange={(event) => setPreference(parseTheme(event.target.value))}
      >
        <option value="system">System</option>
        <option value="light">Light</option>
        <option value="dark">Dark</option>
      </select>
    </label>
  );
}
