/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import React, {useCallback, useEffect, useMemo, useState} from "react";
import {ApplicationContext, IApplicationContextData} from "./AppContext";

interface ContextProviderProps {
  children: React.ReactNode
}

interface IK8sContext {
  context: {
    cluster: string;
    user: string;
  }
  name: string;
}

type K8sContextsResponse = IK8sContext[][] | IK8sContext[];

const ContextProvider = ({children}: ContextProviderProps) => {
  const [appContext, setAppContext] = useState<IApplicationContextData>({
    apiUrl: process.env.NODE_ENV !== 'production' ? "http://localhost:5000/" : "",
    currentK8sContext: "",
    k8sContexts: [],
  });

  const setCurrentContext = useCallback((updates: any) => {
    setAppContext({
      ...appContext,
      ...updates,
    });
  }, [appContext, setAppContext]);

  useEffect(() => {
    fetch(`${appContext.apiUrl}api/v1/contexts`)
      .then<K8sContextsResponse>(res => res.json())
      .then(body => {
        if (body.length > 1) {
          setAppContext({
            ...appContext,
            k8sContexts: (body[0] as IK8sContext[]).map(c => c.name),
            currentK8sContext: (body[1] as IK8sContext).name,
          });
        }
      })
      .catch(err => {
        console.error(err);
      })
  }, []);

  // memoize the full context value
  const contextValue = useMemo(() => ({
    context: appContext,
    setContext: setCurrentContext
  }), [appContext, setCurrentContext])

  return (
    // the Provider gives access to the context to its children
    <ApplicationContext.Provider value={contextValue}>
      {children}
    </ApplicationContext.Provider>
  );
};

export default ContextProvider;
