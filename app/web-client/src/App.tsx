/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import React from "react";
import ContextProvider from "./AppContextProvider";
import { EuiProvider } from "@elastic/eui";
import { Routes, Route } from "react-router-dom";
import { Home } from "./pages/Home";
import { Header } from "./components/Header";
import { Footer } from "./components/Footer";
import { ConstraintTemplates } from "./pages/ConstraintTemplates";
import { Constraints } from "./pages/Constraints";
import { Configurations } from "./pages/Configurations";
import { Error } from "./pages/Error";
import { NotFound } from "./pages/NotFound";
import {theme} from "./theme";
import "./App.scss";

function App() {
  return (
    <EuiProvider
      colorMode="light"
      modify={theme}
    >
      <ContextProvider>
        <Header />
        <Routes>
          <Route path={`/constrainttemplates`}>
            <Route path=":context" element={<ConstraintTemplates />} />
            <Route path="" element={<ConstraintTemplates />} />
          </Route>
          <Route path={`/constraints`}>
            <Route path=":context" element={<Constraints />} />
            <Route path="" element={<Constraints />} />
          </Route>
          <Route path={`/configurations`}>
            <Route path=":context" element={<Configurations />} />
            <Route path="" element={<Configurations />} />
          </Route>
          <Route path={`/error`}>
            <Route path=":context" element={<Error />} />
            <Route path="" element={<Error />} />
          </Route>
          <Route path={`/`} element={<Home />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
        <Footer />
      </ContextProvider>
    </EuiProvider>
  );
}

export default App;
