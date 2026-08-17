// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// One reusable Alpine component, instantiated per constraint via x-data="violationsTable(id)". It
// reads the constraint's violations from its <script> data island, then filters, sorts and
// paginates them entirely client-side. External (not inline) so the page can carry a strict
// Content Security Policy; loaded before the deferred Alpine bundle, so the global function exists
// when Alpine initializes the components.
// A stable, content-derived id for one violation, so a shared deep link keeps pointing at the same
// violation across audits even as others come and go (issue #1324). Two identical violations hash
// the same, which is fine -- they are interchangeable. djb2 over the fields, base36.
// decodeURIComponent throws on a malformed percent sequence; a bad URL fragment must degrade to
// "no match", never throw out of the caller.
function decodeHash(s) {
  try {
    return decodeURIComponent(s);
  } catch (e) {
    return s;
  }
}

function violationId(dataId, r) {
  const s = `${r.enforcementAction}|${r.kind}|${r.namespace}|${r.name}|${r.message}`;
  let h = 5381;
  for (let i = 0; i < s.length; i++) h = (((h << 5) + h) ^ s.charCodeAt(i)) | 0;
  return `${dataId}-${(h >>> 0).toString(36)}`;
}

// Bare http(s) URL matcher, mirrored from urlRE in ssr.go. Keep the two in step: the report and the
// server-rendered views linkify with the Go one, this table linkifies with this one.
const violationUrlRE = /https?:\/\/[^\s<>"]+/g;

// linkifyInto renders a plain violation message into el, turning bare http(s) URLs into links, the
// way the old React UI did (issue #1325). It appends text and <a> nodes and never sets innerHTML, so
// a cluster-controlled message cannot inject markup. It is the client-side twin of linkify in ssr.go
// (used by the download report and the events view); x-effect re-runs it as the table sorts, filters
// and paginates. Trailing sentence punctuation is kept out of the link, as on the server.
function linkifyInto(el, msg) {
  el.textContent = "";
  const s = String(msg ?? "");
  let last = 0;
  for (const m of s.matchAll(violationUrlRE)) {
    const url = m[0].replace(/[.,;:!?)\]}'"]+$/, "");
    const a = document.createElement("a");
    a.href = url;
    a.textContent = url;
    a.target = "_blank";
    a.rel = "noopener noreferrer";
    // append() is variadic: the text before the URL, the link, then the punctuation trimmed off it.
    el.append(s.slice(last, m.index), a, m[0].slice(url.length));
    last = m.index + m[0].length;
  }
  el.append(s.slice(last));
}

function violationsTable(dataId) {
  return {
    columns: [
      { key: "enforcementAction", label: "Action" },
      { key: "namespace", label: "Namespace" },
      { key: "kind", label: "Kind" },
      { key: "name", label: "Name" },
      { key: "message", label: "Message" },
    ],
    rows: [],
    q: "",
    sortKey: "",
    sortDir: "asc",
    page: 0,
    pageSize: 10,
    copiedId: "",
    init() {
      try {
        const raw = JSON.parse(document.getElementById(dataId).textContent) || [];
        this.rows = raw.map((r) => ({ ...r, _id: violationId(dataId, r) }));
      } catch (e) {
        this.rows = [];
      }
      // Deep link: if the URL hash names one of our violations, reveal it. Runs on load and on
      // later hashchange (e.g. someone pastes a link while the page is open). decodeURIComponent
      // throws on a malformed fragment (e.g. "#%"), so guard it -- a bad hash must not kill init.
      const reveal = () => {
        const id = decodeHash(location.hash.slice(1));
        if (id && this.rows.some((r) => r._id === id)) this.focusViolation(id);
      };
      reveal();
      window.addEventListener("hashchange", reveal);
    },
    // Clears any filter/sort so the row sits at its natural index, jumps to its page, expands the
    // collapsed Violations section, then scrolls to it and flashes it.
    focusViolation(id) {
      const idx = this.rows.findIndex((r) => r._id === id);
      if (idx < 0) return;
      this.q = "";
      this.sortKey = "";
      this.page = Math.floor(idx / this.pageSize);
      this.$root.closest("details")?.setAttribute("open", "");
      this.$nextTick(() => {
        const el = document.getElementById(id);
        if (!el) return;
        el.scrollIntoView({ block: "center", behavior: "smooth" });
        el.classList.remove("viol-flash");
        void el.offsetWidth; // reflow so the animation restarts if the row was already flashed
        el.classList.add("viol-flash");
      });
    },
    copyLink(row) {
      const url = location.origin + location.pathname + location.search + "#" + row._id;
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(url).then(
          () => {
            this.copiedId = row._id;
            setTimeout(() => {
              if (this.copiedId === row._id) this.copiedId = "";
            }, 1500);
          },
          () => (location.hash = row._id),
        );
      } else {
        location.hash = row._id; // insecure context: no clipboard, put it in the address bar instead
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
    // Filter + pager only appear once the list needs more than one page; a filter box over a
    // handful of rows is noise. Gate on the full count against the default page size (10) -- a
    // fixed threshold, not the mutable pageSize, so raising Rows to 50 cannot hide the controls
    // you are using, and so a filter that narrows to a few rows cannot hide the box you type in.
    get showControls() {
      return this.rows.length > 10;
    },
    get countLabel() {
      return this.q.trim() ? `${this.total} of ${this.rows.length} match` : `${this.rows.length} violations`;
    },
    get pageCount() {
      return Math.max(1, Math.ceil(this.total / this.pageSize));
    },
    get paged() {
      const start = Math.min(this.page, this.pageCount - 1) * this.pageSize;
      return this.filtered.slice(start, start + this.pageSize);
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
