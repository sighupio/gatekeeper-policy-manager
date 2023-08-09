/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export function scrollToElement(hash: string, smooth: boolean = false) {
  const element = document.querySelector(hash.replace(/:|\./g, '\\$&'));

  if (!element) {
    return;
  }

  element?.firstElementChild?.classList.toggle("highlighted");

  if (smooth) {
    element.scrollIntoView({ behavior: "smooth", block: "center" });
  } else {
    element.scrollIntoView({ block: "center" });
  }

  setTimeout(() => {
    element?.firstElementChild?.classList.toggle("highlighted");
  }, 1000);
}
