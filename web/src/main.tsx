import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./styles.css";
import "./workspace.css";
import "./infrastructure.css";
import "./console.css";
import "./paper.css";
import "./jobs.css";
import "./remediation.css";
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
