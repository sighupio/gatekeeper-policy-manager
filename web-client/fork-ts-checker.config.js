/**
 * Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

module.exports = {
  logger: {
    log: (message) => console.log(message),
    error: (message) => console.error(message),
  },
};
