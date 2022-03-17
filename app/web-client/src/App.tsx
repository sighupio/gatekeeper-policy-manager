/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import React from 'react';
import './App.css';
import {ApplicationContext} from "./AppContext";
import {
    Routes,
    Route,
} from "react-router-dom";
import {Home} from "./pages/Home";
import {Header} from "./components/Header";
import {Footer} from "./components/Footer";
import {ConstraintTemplates} from "./pages/ConstraintTemplates";
import {Constraints} from "./pages/Constraints";
import {Configurations} from "./pages/Configurations";
import "fury-design-system/dist/eui_theme_fury_community.css";

function App() {

  const applicationContext = {
    apiUrl: process.env.NODE_ENV !== 'production' ? "http://localhost:5000/" : "",
  };

  return (
    <>
      <Header />
      <ApplicationContext.Provider value={applicationContext}>
        <Routes>
          <Route path={`/`} element={<Home />} />
          <Route path={`/constrainttemplates`} element={<ConstraintTemplates />} />
          <Route path={`/constraints`} element={<Constraints />} />
          <Route path={`/configurations`} element={<Configurations />} />
          <Route path="*" element={<></>} />
        </Routes>
      </ApplicationContext.Provider>
      <Footer />
    </>
  );
}

export default App;
