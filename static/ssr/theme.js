// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Applies the saved theme before first paint so there is no light/dark flash. Loaded synchronously
// in <head>, so it runs before the deferred Alpine bundle reads data-theme. External (not inline)
// so the page can carry a strict Content Security Policy with no script-src 'unsafe-inline'.
(function () {
  var t = localStorage.getItem("gpm-theme");
  if (t) document.documentElement.setAttribute("data-theme", t);
})();
