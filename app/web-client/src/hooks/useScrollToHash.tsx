/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { useEffect } from 'react';
import {scrollToElement} from "../utils";

export default function useScrollToHash(hash: string, deps: any[]) {
  useEffect(() => {
    if (hash) {
      scrollToElement(hash, false);
    } else {
      window.scrollTo(0, 0);
    }
  }, [...deps])
}
