// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Applies the saved theme before first paint so there is no light/dark flash. Only "light" and
// "dark" are pinned; anything else (or nothing stored) means "system", which follows the OS via the
// prefers-color-scheme media query. Loaded synchronously in <head>, so it runs before the deferred
// Alpine bundle. External (not inline) so the page can carry a strict Content Security Policy.
(function () {
  var t = localStorage.getItem("gpm-theme");
  if (t === "light" || t === "dark") document.documentElement.setAttribute("data-theme", t);
})();

// The theme toggle cycles system -> light -> dark. "system" clears the override so the page follows
// the OS setting; light and dark pin data-theme. The button shows the current mode, so "follow the
// system theme" is a visible, selectable state rather than an invisible default.
function themeToggle() {
  return {
    order: ["system", "light", "dark"],
    icons: { system: "◐", light: "☀", dark: "☾" },
    mode: localStorage.getItem("gpm-theme") || "system",
    get icon() {
      return this.icons[this.mode];
    },
    get title() {
      return "Theme: " + this.mode + " (click to change)";
    },
    cycle() {
      this.mode = this.order[(this.order.indexOf(this.mode) + 1) % this.order.length];
      if (this.mode === "system") {
        localStorage.removeItem("gpm-theme");
        document.documentElement.removeAttribute("data-theme");
      } else {
        localStorage.setItem("gpm-theme", this.mode);
        document.documentElement.setAttribute("data-theme", this.mode);
      }
    },
  };
}
