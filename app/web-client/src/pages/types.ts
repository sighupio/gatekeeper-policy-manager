/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export interface ISideNavItem {
  name: string;
  id: string;
  href?: string;
  disabled?: boolean;
  isSelected?: boolean;
  onClick?: () => void;
}

export interface ISideNav {
  name: string;
  id: string;
  items: ISideNavItem[];
}
