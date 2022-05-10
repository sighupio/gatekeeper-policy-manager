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
} from "fury-design-system";
import { useEffect } from "react";
import { useLocation } from "react-router-dom";
import { ErrorPageState } from "./types";

function ErrorComponent() {
  const { state } = useLocation();

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
