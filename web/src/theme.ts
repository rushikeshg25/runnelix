export type ThemePreference = "system" | "light" | "dark";
export const themeStorageKey = "runnelix.theme";

export function parseTheme(value: string | null): ThemePreference {
  return value === "light" || value === "dark" ? value : "system";
}

export function resolveTheme(preference: ThemePreference, systemDark: boolean) {
  return preference === "system" ? (systemDark ? "dark" : "light") : preference;
}

export function readTheme(): ThemePreference {
  try {
    return parseTheme(localStorage.getItem(themeStorageKey));
  } catch {
    return "system";
  }
}

export function applyTheme(preference: ThemePreference, systemDark: boolean) {
  const theme = resolveTheme(preference, systemDark);
  document.documentElement.dataset.theme = theme;
  return theme;
}
