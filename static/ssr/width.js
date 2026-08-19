// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Applies the saved layout width before first paint. Every view is a fresh server render, so without
// this the page would start centred and snap to full width on every navigation. Only "full" is
// stored; anything else means the default, which caps the page and centres it. Loaded synchronously
// in <head> like theme.js, and external (not inline) so the page can carry a strict CSP.
(function () {
  document.documentElement.toggleAttribute(
    "data-full-width",
    localStorage.getItem("gpm-width") === "full",
  );
})();

// Two states, so a plain toggle rather than the theme's cycle. The icon shows the layout in force and
// the title says what a click does, matching themeToggle.
function widthToggle() {
  return {
    full: localStorage.getItem("gpm-width") === "full",
    get title() {
      return this.full
        ? "Layout: full width (click to centre it)"
        : "Layout: centred (click to use the whole window)";
    },
    toggle() {
      this.full = !this.full;
      // A boolean state, so a valueless attribute: data-theme next door carries an enum and needs
      // its value, this one only needs to be there or not.
      document.documentElement.toggleAttribute("data-full-width", this.full);
      if (this.full) localStorage.setItem("gpm-width", "full");
      else localStorage.removeItem("gpm-width");
    },
  };
}
