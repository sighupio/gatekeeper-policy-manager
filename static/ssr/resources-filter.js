// Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Filters the Resources view. The rows are server-rendered, so this only hides them: no re-render, no
// data island, and the page still works with the filter untouched. A namespace card whose rows all
// hide goes with them, and so does its sidebar entry, or the page would keep empty headings around.
function resourcesFilter() {
  return {
    q: "",
    hidden: 0,
    total: 0,
    copiedId: "",
    init() {
      onShareLink((id) => {
        const row = id && document.getElementById(id);
        // Only a row: the namespace cards carry ids too, and the sidebar links to them. Revealing
        // one of those would clear the filter for what is really just in-page navigation.
        if (row && row.classList.contains("event-row")) this.focusRow(row);
      });
    },
    // Clears the filter first: a link can arrive while a filter is hiding the row it names.
    focusRow(row) {
      this.q = "";
      this.apply();
      row.open = true;
      this.$nextTick(() => {
        row.scrollIntoView({ block: "center", behavior: "smooth" });
        row.classList.remove("res-flash");
        void row.offsetWidth; // reflow, so the flash restarts on a repeat visit to the same link
        row.classList.add("res-flash");
      });
    },
    apply() {
      const needle = this.q.trim().toLowerCase();
      this.total = 0;
      this.hidden = 0;

      for (const card of document.querySelectorAll(".nscard")) {
        let shown = 0;
        for (const row of card.querySelectorAll(".event-row")) {
          const match =
            !needle || row.dataset.search.toLowerCase().includes(needle);
          row.hidden = !match;
          this.total++;
          match ? shown++ : this.hidden++;
        }
        card.hidden = shown === 0;
        const entry = document.querySelector(
          `.nsnav a[data-ns="${CSS.escape(card.dataset.ns)}"]`,
        );
        if (entry) entry.hidden = shown === 0;
      }
    },
  };
}
