/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiButton,
  EuiFlexGroup,
  EuiFlexItem,
  EuiPage,
  EuiSpacer,
  EuiText,
} from "fury-design-system";

function HomeComponent() {
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
            <h1>Welcome!</h1>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiText textAlign="center">
            <h2>
              Gatekeeper Policy Manager is a simple to use web-based tool to see
              the policies deployed in your cluster and their status
            </h2>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiButton
            href="/constraints"
            iconSide="right"
            iconType="arrowRight"
            aria-label="Next"
          >
            See Constraints status
          </EuiButton>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="xxl" />
    </EuiPage>
  );
}

export default HomeComponent;
