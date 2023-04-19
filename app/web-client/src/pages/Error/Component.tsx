/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiButton,
  EuiCallOut,
  EuiFlexGroup,
  EuiFlexItem,
  EuiIcon,
  EuiPage,
  EuiSpacer,
  EuiText,
} from "@elastic/eui";
import { useContext, useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ErrorPageState } from "./types";
import { ApplicationContext } from "../../AppContext";

function ErrorComponent() {
  const { state } = useLocation();
  const navigate = useNavigate();
  const [initialContext, setInitialContext] = useState<string>();
  const appContextData = useContext(ApplicationContext);

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
  }, [appContextData.context.currentK8sContext])

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
          <EuiButton
            href={`/${(state as ErrorPageState)?.entity ?? ""}`}
            iconSide="right"
            iconType="arrowRight"
            aria-label="Next"
          >
            Go back
          </EuiButton>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="xxl" />
    </EuiPage>
  );
}

export default ErrorComponent;
