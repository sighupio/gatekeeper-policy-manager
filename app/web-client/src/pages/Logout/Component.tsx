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
} from "@elastic/eui";

function LogoutComponent() {
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
            <h2>You've been successfully logged out!</h2>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiButton
            href="/"
            iconSide="right"
            iconType="arrowRight"
            aria-label="Next"
          >
            Go to home
          </EuiButton>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="xxl" />
    </EuiPage>
  );
}

export default LogoutComponent;
