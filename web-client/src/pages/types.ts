/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { RenderItem } from "@elastic/eui/src/components/side_nav/side_nav_item";
import { ReactElement } from "react";

export interface ISideNavItem {
  name: string;
  id: string;
  href?: string;
  disabled?: boolean;
  renderItem?: RenderItem<any>;
  icon?: ReactElement;
  isSelected?: boolean;
  onClick?: () => void;
}

export interface ISideNav {
  name: string;
  id: string;
  items: ISideNavItem[];
}

export interface BackendError {
  action: string;
  description: string;
  error: string;
}
