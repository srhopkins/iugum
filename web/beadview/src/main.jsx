import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App.jsx";
import TicketPage from "./components/TicketPage.jsx";
import "./styles/global.css";

// /bead/<project>/<id> is the full-page ticket. Everything else is the app.
const ticket = /^\/bead\/([^/]+)\/(.+)$/.exec(window.location.pathname);

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <div className="beads-viewer-root">
      {ticket ? (
        <TicketPage
          project={decodeURIComponent(ticket[1])}
          id={decodeURIComponent(ticket[2])}
        />
      ) : (
        <App />
      )}
    </div>
  </React.StrictMode>
);
