import { createRoot } from "react-dom/client";

import App from "./App";
import "./i18n";
import "./styles.css";

document.documentElement.setAttribute("data-theme", "dr600ab");

const root = createRoot(document.getElementById("root") as HTMLElement);
root.render(<App />);
