/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiButton,
  EuiFlexGroup,
  EuiFlexItem,
  EuiImage,
  EuiPage,
  EuiSpacer,
  EuiText,
} from "@elastic/eui";
import logo from "../../assets/gpm-logo.svg";
import {useContext} from "react";
import {ApplicationContext} from "../../AppContext";

function HomeComponent() {
  const appContextData = useContext(ApplicationContext);

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
          <EuiImage style={{width: 100}} src={logo} alt="gpm"/>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiText textAlign="center">
            <h1>Welcome!</h1>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiText textAlign="center">
            <h2 style={{fontWeight: 300}}>
              Gatekeeper Policy Manager is a simple to use web-based tool to see
              the policies deployed in your cluster and their status
            </h2>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiButton
            href={`/constraints/${appContextData.context.currentK8sContext}`}
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
