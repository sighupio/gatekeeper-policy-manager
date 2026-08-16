// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// One reusable Alpine component, instantiated per constraint via x-data="violationsTable(id)". It
// reads the constraint's violations from its <script> data island, then filters, sorts and
// paginates them entirely client-side. External (not inline) so the page can carry a strict
// Content Security Policy; loaded before the deferred Alpine bundle, so the global function exists
// when Alpine initializes the components.
function violationsTable(dataId) {
  return {
    columns: [
      { key: "enforcementAction", label: "Action" },
      { key: "kind", label: "Kind" },
      { key: "namespace", label: "Namespace" },
      { key: "name", label: "Name" },
      { key: "message", label: "Message" },
    ],
    rows: [],
    q: "",
    sortKey: "",
    sortDir: "asc",
    page: 0,
    pageSize: 10,
    init() {
      try {
        this.rows = JSON.parse(document.getElementById(dataId).textContent) || [];
      } catch (e) {
        this.rows = [];
      }
    },
    // ponytail: re-filters/re-sorts on every read; fine at Gatekeeper's audit limit (~20 rows).
    // If someone raises --constraint-violations-limit into the thousands, memoize on q/sort.
    get filtered() {
      let out = this.rows;
      const q = this.q.trim().toLowerCase();
      if (q) {
        out = out.filter((r) =>
          this.columns.some((c) => String(r[c.key] ?? "").toLowerCase().includes(q)),
        );
      }
      if (this.sortKey) {
        const dir = this.sortDir === "asc" ? 1 : -1;
        out = [...out].sort((a, b) =>
          dir *
          String(a[this.sortKey] ?? "").localeCompare(String(b[this.sortKey] ?? ""), undefined, {
            numeric: true,
            sensitivity: "base",
          }),
        );
      }
      return out;
    },
    get total() {
      return this.filtered.length;
    },
    get pageCount() {
      return Math.max(1, Math.ceil(this.total / this.pageSize));
    },
    get paged() {
      const start = Math.min(this.page, this.pageCount - 1) * this.pageSize;
      return this.filtered.slice(start, start + this.pageSize);
    },
    get from() {
      return this.total === 0 ? 0 : Math.min(this.page, this.pageCount - 1) * this.pageSize + 1;
    },
    get to() {
      return Math.min(this.total, (Math.min(this.page, this.pageCount - 1) + 1) * this.pageSize);
    },
    sort(key) {
      if (this.sortKey === key) {
        this.sortDir = this.sortDir === "asc" ? "desc" : "asc";
      } else {
        this.sortKey = key;
        this.sortDir = "asc";
      }
      this.page = 0;
    },
  };
}
