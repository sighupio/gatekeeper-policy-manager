/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { MutableRefObject, useEffect } from "react";

export default function useCurrentElementInView(
  refs: MutableRefObject<HTMLDivElement[]>,
  cb: (id: string) => any,
  offset = 50
) {
  const onScroll = () => {
    const elementVisible = refs.current.filter((element) => {
      const top = element.getBoundingClientRect().top;

      return top + offset >= 0 && top - offset <= window.innerHeight;
    });

    if (elementVisible.length > 0) {
      cb(elementVisible[0].id);
    }
  };

  useEffect(() => {
    document.addEventListener("scroll", onScroll, true);
    return () => document.removeEventListener("scroll", onScroll, true);
  }, []);
}
