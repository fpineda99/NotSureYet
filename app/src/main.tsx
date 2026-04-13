import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";

// Disable right-click context menu everywhere except the editor
document.addEventListener("contextmenu", (e) => {
  const target = e.target as HTMLElement;
  // Allow right-click inside the TipTap editor for autocorrect/spellcheck
  if (target.closest(".tiptap")) return;
  // Allow right-click on session items for context menu
  if (target.closest(".session-items li")) return;
  e.preventDefault();
});

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
