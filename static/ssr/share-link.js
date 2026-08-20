// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Deep links to a single row, shared by the violations table and the Resources view: both address a
// row by URL fragment, so a change to how that works should only have to be made once.

// decodeURIComponent throws on a malformed percent sequence; a bad URL fragment must degrade to
// "no match", never throw out of the caller.
function decodeHash(s) {
  try {
    return decodeURIComponent(s);
  } catch (e) {
    return s;
  }
}

// Hands the decoded fragment to fn now and on every later hashchange -- someone can paste a link
// into a page that is already open.
function onShareLink(fn) {
  const run = () => fn(decodeHash(location.hash.slice(1)));
  run();
  window.addEventListener("hashchange", run);
}

// Copies a deep link to one row, and flips the calling component's copiedId so its button can show
// a tick.
function copyShareLink(component, id) {
  const url = `${location.origin}${location.pathname}${location.search}#${id}`;
  const tick = () => {
    component.copiedId = id;
    setTimeout(() => {
      if (component.copiedId === id) component.copiedId = "";
    }, 1500);
  };
  // There is no Clipboard API outside a secure context, which covers any GPM served over plain HTTP
  // on a hostname. Putting the link in the address bar leaves it copyable by hand.
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(url).then(tick, () => (location.hash = id));
  } else {
    location.hash = id;
  }
}
