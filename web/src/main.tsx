import { createRoot } from "react-dom/client";
import { SessionBoundary } from "./SessionBoundary";
import "./style.css";
import { ThemeProvider } from "./Theme";
import { applyTheme, readTheme } from "./theme";
applyTheme(readTheme(), matchMedia("(prefers-color-scheme: dark)").matches);
createRoot(document.getElementById("root")!).render(
  <ThemeProvider>
    <SessionBoundary />
  </ThemeProvider>,
);
