/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { MutableRefObject, useEffect } from "react";

export default function useCurrentElementInView(
  refs: MutableRefObject<HTMLDivElement[]>,
  cb: (id: string) => any,
  offset = 0
) {
  const observer = new IntersectionObserver((entries, observer) => {
    const elementVisible = entries.filter((element) => {
      const top = element.boundingClientRect.top;

      return top + offset >= 0 && top - offset <= window.innerHeight;
    })

    if (elementVisible.length > 0) {
      cb(elementVisible[0].target.id);
    }

    observer.disconnect();
  });

  const onScroll = () => {
    refs.current.forEach(el => {
      observer.observe(el);
    })
  };

  useEffect(() => {
    document.addEventListener("scroll", onScroll, true);
    return () => document.removeEventListener("scroll", onScroll, true);
  }, []);
}
