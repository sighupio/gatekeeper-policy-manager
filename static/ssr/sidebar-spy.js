// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Sidebar scroll-spy for the Constraints and Constraint Templates views: highlights the nav entry
// whose section is currently in view, so the long lists show "you are here" as you scroll. Loaded
// on every page but a no-op where there is no sidebar. Class-only -- it never writes location.hash,
// so a shared deep link (e.g. issue #1324, a link to one violation) is not clobbered.
(function () {
  const links = Array.from(document.querySelectorAll(".sidebar-nav a[href^='#']"));
  if (links.length < 2) return;

  // decodeURIComponent throws on a malformed fragment; a crafted hash must not kill the scroll-spy.
  const decodeHash = (s) => {
    try {
      return decodeURIComponent(s);
    } catch (e) {
      return s;
    }
  };

  const linkFor = new Map(); // section element -> nav link
  const sections = []; // in DOM order, which mirrors the sidebar order
  for (const a of links) {
    const el = document.getElementById(decodeHash(a.getAttribute("href").slice(1)));
    if (el) {
      linkFor.set(el, a);
      sections.push(el);
    }
  }
  if (!sections.length) return;

  let active = null;
  const setActive = (a) => {
    if (a === active) return;
    if (active) active.classList.remove("active");
    if (a) a.classList.add("active");
    active = a;
  };

  // The topmost section that is on screen is the active one. rootMargin pulls the top band down
  // past the sticky navbar and treats a section as current once it reaches the upper viewport.
  const onScreen = new Set();
  const io = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (e.isIntersecting) onScreen.add(e.target);
        else onScreen.delete(e.target);
      }
      const top = sections.find((el) => onScreen.has(el));
      if (top) setActive(linkFor.get(top));
    },
    { rootMargin: "-80px 0px -60% 0px", threshold: 0 },
  );
  sections.forEach((el) => io.observe(el));

  // Highlight something from the first paint: the deep-linked section if the URL carries a hash,
  // otherwise the first entry. The observer corrects it on the first scroll.
  const hashEl = location.hash && document.getElementById(decodeHash(location.hash.slice(1)));
  setActive(linkFor.get(hashEl) || linkFor.get(sections[0]));
})();
