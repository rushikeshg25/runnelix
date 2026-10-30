import { createRoot } from "react-dom/client";
import { SessionBoundary } from "./SessionBoundary";
import "./style.css";
createRoot(document.getElementById("root")!).render(<SessionBoundary />);
