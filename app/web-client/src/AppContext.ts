/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {createContext} from "react";

export interface IApplicationContextData {
  apiUrl: string;
  k8sContexts: string[];
  currentK8sContext: string;
  authEnabled: boolean;
}

export interface IApplicationContext {
  context: IApplicationContextData;
  setContext?: (context: Partial<IApplicationContextData>) => void;
}

export const ApplicationContext = createContext<IApplicationContext>({
  context: {
    apiUrl: "",
    k8sContexts: [],
    currentK8sContext: "",
    authEnabled: false,
  },
});
