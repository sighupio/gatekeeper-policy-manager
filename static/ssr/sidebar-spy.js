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
  const sectionFor = new Map(); // and back, to place the current mark in the list
  const sections = []; // in DOM order, which mirrors the sidebar order
  for (const a of links) {
    const el = document.getElementById(decodeHash(a.getAttribute("href").slice(1)));
    if (el) {
      linkFor.set(el, a);
      sectionFor.set(a, el);
      sections.push(el);
    }
  }
  if (!sections.length) return;

  // The sidebar is its own scroll box (app.css .sidebar caps it to the window and scrolls inside),
  // so on a long list the marked entry can sit outside it and the reader sees no mark at all. Bring
  // it back into that box by the smallest amount, and only that box: scrollIntoView would scroll the
  // window as well, moving the very page position the mark is computed from.
  const box = links[0].closest(".sidebar");
  const keepMarkVisible = (a) => {
    if (!box || box.scrollHeight <= box.clientHeight) return;
    const edge = 8;
    const b = box.getBoundingClientRect();
    const r = a.getBoundingClientRect();
    if (r.top < b.top + edge) box.scrollTop -= b.top + edge - r.top;
    else if (r.bottom > b.bottom - edge) box.scrollTop += r.bottom - b.bottom + edge;
  };

  let active = null;
  const setActive = (a) => {
    if (a === active) return;
    if (active) {
      active.classList.remove("active");
      active.removeAttribute("aria-current");
    }
    if (a) {
      a.classList.add("active");
      // The mark was a colour and nothing else, so a screen reader could not tell which entry it is.
      a.setAttribute("aria-current", "true");
      keepMarkVisible(a);
    }
    active = a;
  };

  const doc = document.documentElement;

  // A section becomes current when its top passes the reading line, so the marked entry is the
  // section you are inside. The line sits below the sticky topbar, above .card's scroll-margin-top
  // (76px) so a clicked card lands on the marked side of it.
  //
  // Near the end of the page there is no scroll left to lift the last sections up to that line, so
  // the line comes down to meet them instead, by exactly the scrolling the page can no longer do.
  // Without this the whole tail is unreachable: it kept an earlier entry marked through the last
  // screenful and snapped to the last entry at the final pixel, skipping the entries between.
  const ANCHOR = 80;
  const readingLine = () => {
    const maxScroll = doc.scrollHeight - innerHeight;
    // A page that cannot scroll has one reading position, its top.
    if (maxScroll <= 0) return ANCHOR;

    const remaining = Math.max(0, maxScroll - scrollY);
    const shortfall = Math.max(0, innerHeight - ANCHOR - remaining);
    // At the foot, slide the whole way: there is no scrolling left to bring the last sections up, so
    // anything less leaves them unmarkable. Reading nothing but the window here also keeps the line
    // still when a <details> expands and the browser's scroll anchoring moves scrollY under us.
    if (remaining === 0) return ANCHOR + shortfall;

    // Above the foot, slide no further than the reader has actually scrolled, or the top of a page
    // that barely scrolls would open with an entry several sections ahead already marked.
    return ANCHOR + Math.min(shortfall, Math.max(0, scrollY));
  };

  // A click states which entry the reader wants, and it outranks the position rule until they scroll
  // for themselves. Landing on one of the last entries parks the page at the end, where the reading
  // line has nothing to answer with but the last section, so position alone would mark the wrong one.
  let pinned = null;

  // Sections are in DOM order, so the last one whose top has passed the line is the current one.
  //
  // The mark never moves backwards unless the reader scrolled back. Expanding a <details> grows the
  // page underneath them, which takes it off the foot and slides the line up; the browser's scroll
  // anchoring then fires a scroll event, and the mark would jump to a card above the one being read.
  // Nothing they can see moved, so neither should the mark.
  let lastScroll = scrollY;
  const update = () => {
    if (pinned) return;
    const line = readingLine();
    let found = sections[0];
    for (const el of sections) {
      if (el.getBoundingClientRect().top - 1 > line) break;
      found = el;
    }
    const wentBack = sections.indexOf(found) < sections.indexOf(active ? sectionFor.get(active) : found);
    const scrolledUp = scrollY < lastScroll;
    lastScroll = scrollY;
    if (wentBack && !scrolledUp) return;
    setActive(linkFor.get(found));
  };

  // One update per frame. A plain scroll listener replaced an IntersectionObserver here: the
  // observer reports a section crossing a fixed band, which is the wrong question once the page
  // cannot scroll far enough to put a section there.
  let ticking = false;
  const onScroll = () => {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => {
      ticking = false;
      update();
    });
  };
  addEventListener("scroll", onScroll, { passive: true });
  // The line is measured against the window, and cards reflow at a new width.
  addEventListener("resize", onScroll, { passive: true });
  // Fonts and images can settle after this runs, moving the cards without any scrolling.
  addEventListener("load", onScroll);

  links.forEach((a) =>
    a.addEventListener("click", (e) => {
      // Ctrl/Cmd/Shift-click opens the section in a new tab or window and leaves this page exactly
      // where it was. Pinning there would move the mark to a section the reader never went to, and
      // the pin would hold it wrong until they scrolled.
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.defaultPrevented) return;
      pinned = a;
      setActive(a);
    }),
  );

  // Any scrolling the reader starts themselves releases the pin: the wheel, a touch drag, the
  // keyboard, or a pointer on the scrollbar. pointerdown also fires on the way into a nav click, and
  // that is the right order -- it releases the old pin just before the click sets the new one.
  //
  // Releasing must not recompute. Nothing has moved yet, so recomputing on the pointerdown of a nav
  // click marks whatever sits under the reading line for one frame before the click marks the entry
  // that was clicked: a visible flick to a later entry and back, worst on a tall window where the
  // line is far down the page. Whatever the reader does next, scroll or click, updates the mark.
  const unpin = () => {
    pinned = null;
  };
  for (const ev of ["wheel", "touchstart", "pointerdown", "keydown"]) {
    addEventListener(ev, unpin, { passive: true });
  }

  // Back, Forward and an address-bar fragment move the page without any of the events above, so the
  // pin would hold the old mark over a section the reader has left. Recomputing is right here, in
  // contrast to unpin: the page really has moved.
  for (const ev of ["hashchange", "popstate"]) {
    addEventListener(ev, () => {
      // A nav click changes the fragment too, and that one must keep its pin -- the fragment it
      // lands on is the entry that was clicked. Anything else (Back, Forward, an address-bar edit)
      // names a different section, so release and recompute: the page really has moved.
      const wanted = pinned && decodeHash(pinned.getAttribute("href").slice(1));
      if (wanted && wanted === decodeHash(location.hash.slice(1))) return;
      pinned = null;
      onScroll();
    });
  }

  update();
})();
