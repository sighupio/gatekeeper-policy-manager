// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Reusable Alpine component for the home dashboard's sortable tables (clusters, violations by
// constraint). It reads its rows from a <script> data island and sorts them client-side. External
// (not inline) so the page can keep a strict Content Security Policy; loaded before the deferred
// Alpine bundle, so the global function exists when Alpine initializes the components.
function dashboardTable(dataId, sortKey, sortDir) {
  return {
    rows: [],
    sortKey: sortKey || "",
    sortDir: sortDir || "asc",
    init() {
      try {
        this.rows = JSON.parse(document.getElementById(dataId).textContent) || [];
      } catch (e) {
        this.rows = [];
      }
    },
    // ponytail: re-sorts on every read; a fleet holds tens of clusters, not thousands.
    get sorted() {
      if (!this.sortKey) return this.rows;
      const key = this.sortKey;
      const dir = this.sortDir === "asc" ? 1 : -1;
      return [...this.rows].sort(
        (a, b) =>
          dir *
          String(a[key] ?? "").localeCompare(String(b[key] ?? ""), undefined, {
            numeric: true,
            sensitivity: "base",
          }),
      );
    },
    sort(key) {
      if (this.sortKey === key) {
        this.sortDir = this.sortDir === "asc" ? "desc" : "asc";
      } else {
        this.sortKey = key;
        this.sortDir = "asc";
      }
    },
    ind(key) {
      return this.sortKey === key ? (this.sortDir === "asc" ? "▲" : "▼") : "";
    },
    aria(key) {
      return this.sortKey === key ? (this.sortDir === "asc" ? "ascending" : "descending") : "none";
    },
  };
}

// Sorts a grid "table" whose rows are server-rendered <details> elements (the Events view) by
// reordering the DOM rather than re-rendering, so each row's expandable detail and open state
// survive. Each row carries data-<key> attributes; sort(key) reorders the rows by that attribute.
function sortableGrid(rowSelector) {
  return {
    sortKey: "",
    sortDir: "asc",
    sort(key) {
      if (this.sortKey === key) {
        this.sortDir = this.sortDir === "asc" ? "desc" : "asc";
      } else {
        this.sortKey = key;
        this.sortDir = "asc";
      }
      const dir = this.sortDir === "asc" ? 1 : -1;
      const rows = Array.from(this.$root.querySelectorAll(rowSelector));
      rows.sort(
        (a, b) =>
          dir *
          String(a.dataset[key] ?? "").localeCompare(String(b.dataset[key] ?? ""), undefined, {
            numeric: true,
            sensitivity: "base",
          }),
      );
      rows.forEach((r) => this.$root.appendChild(r));
    },
    ind(key) {
      return this.sortKey === key ? (this.sortDir === "asc" ? "▲" : "▼") : "";
    },
    aria(key) {
      return this.sortKey === key ? (this.sortDir === "asc" ? "ascending" : "descending") : "none";
    },
  };
}

// A ticking "updated Ns ago" label for the dashboard. The page is server-rendered from a cache that
// can be up to a few seconds old (and Gatekeeper's audit lags ~a minute), so show the data's age
// rather than pretend it is live. unixMs is when the server fetched the data.
function updatedAgo(unixMs) {
  return {
    at: unixMs,
    now: Date.now(),
    init() {
      setInterval(() => (this.now = Date.now()), 1000);
    },
    get text() {
      const s = Math.max(0, Math.round((this.now - this.at) / 1000));
      if (s < 5) return "updated just now";
      if (s < 60) return "updated " + s + "s ago";
      const m = Math.floor(s / 60);
      return "updated " + m + "m " + (s % 60) + "s ago";
    },
    get title() {
      return "Fetched " + new Date(this.at).toLocaleString();
    },
  };
}
