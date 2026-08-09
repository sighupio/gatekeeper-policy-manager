/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiButton,
  EuiFlexGroup,
  EuiFlexItem,
  EuiIcon,
  EuiPage,
  EuiSpacer,
  EuiText,
  EuiCallOut,
} from "@elastic/eui";
import { useContext, useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ErrorPageState } from "./types";
import { ApplicationContext } from "../../AppContext";
import { appPath } from "../../utils";

function ErrorComponent() {
  const { state } = useLocation();
  const navigate = useNavigate();
  const [initialContext, setInitialContext] = useState<string>();
  const appContextData = useContext(ApplicationContext);

  // The backend sets login_url only when a sign-in fixes the error. So the button shows up for an
  // expired session and stays out of the way for every other failure. entity holds the path of the
  // page that failed: the Go-back button returns there, and ?next= sends the user back after login.
  const errorState = state as ErrorPageState | null;
  const entity = errorState?.entity;
  const loginUrl = errorState?.error?.login_url;
  const loginHref =
    loginUrl && entity
      ? `${loginUrl}?next=${encodeURIComponent(appPath(entity))}`
      : loginUrl;

  useEffect(() => {
    if (
      initialContext === undefined &&
      appContextData.context.currentK8sContext !== undefined
    ) {
      setInitialContext(appContextData.context.currentK8sContext);
    }

    if (
      initialContext !== undefined &&
      appContextData.context.currentK8sContext !== initialContext
    ) {
      navigate("/");
    }
  }, [appContextData.context.currentK8sContext]);

  return (
    <EuiPage
      paddingSize="s"
      direction="column"
      restrictWidth={600}
      style={{
        height: "85vh",
      }}
      grow={true}
      className="gpm-page"
    >
      <EuiFlexGroup
        justifyContent="center"
        alignItems="center"
        direction="column"
      >
        <EuiFlexItem grow={false}>
          <EuiText textAlign="center">
            <h1>
              <EuiIcon
                style={{ marginRight: 10, color: "red" }}
                type="alert"
                size="xxl"
              />
              Error
            </h1>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiText textAlign="center">
            <h2>{(state as ErrorPageState)?.error?.error}</h2>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiText textAlign="center">
            <h4>{(state as ErrorPageState)?.error?.action}</h4>
          </EuiText>
        </EuiFlexItem>
        <EuiCallOut title="Error details" color="danger" iconType="error">
          <p>{(state as ErrorPageState)?.error?.description}</p>
        </EuiCallOut>
        <EuiFlexItem grow={false}>
          <EuiFlexGroup justifyContent="center" gutterSize="s">
            {loginUrl && (
              <EuiFlexItem grow={false}>
                {/* A full page load, not client-side routing: signing in leaves the app for the
                    identity provider and comes back through the backend. */}
                <EuiButton
                  href={loginHref}
                  fill
                  iconType="push"
                  aria-label="Log in"
                >
                  Log in
                </EuiButton>
              </EuiFlexItem>
            )}
            <EuiFlexItem grow={false}>
              <EuiButton
                href={appPath(entity || "/")}
                iconSide="right"
                iconType="arrowRight"
                aria-label="Next"
              >
                Go back
              </EuiButton>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="xxl" />
    </EuiPage>
  );
}

export default ErrorComponent;
