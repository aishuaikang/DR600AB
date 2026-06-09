import { createRoot } from "react-dom/client";

import App from "./App";
import { installPageGestureGuards } from "./utils/pageGestures";
import "./i18n";
import "./styles.css";

document.documentElement.setAttribute("data-theme", "dr600ab");
installPageGestureGuards();

const root = createRoot(document.getElementById("root") as HTMLElement);
root.render(<App />);

window.setTimeout(() => {
  document.documentElement.classList.add("app-hydrated");
  window.setTimeout(() => {
    document.querySelector(".app-boot-loader")?.remove();
  }, 420);
}, 80);
