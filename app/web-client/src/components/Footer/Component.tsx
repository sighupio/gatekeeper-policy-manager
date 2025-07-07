/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiFlexGroup,
  EuiFlexItem,
  EuiIcon,
  EuiLink,
  EuiSpacer,
  EuiText,
} from "@elastic/eui";
import githubLogo from "../../assets/github-logo.svg";
import "./Style.scss";

function FooterComponent() {
  return (
    <footer className="gpm-footer">
      <EuiFlexGroup justifyContent="spaceBetween">
        <EuiFlexItem grow={false}>
          <EuiText size="s" className="dynamic">
            <p>
              <strong>Gatekeeper Policy Manager v1.0.14</strong>
            </p>
          </EuiText>
          <EuiText size="s">
            <p>A simple to use web-based Gatekeeper policies manager</p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p>
              <EuiIcon type="heart" color="red" /> Proud part of the{" "}
              <EuiLink href="https://docs.sighup.io/" target="_blank">
                SIGHUP Distribution
              </EuiLink>
            </p>
          </EuiText>
          <EuiFlexGroup justifyContent="flexStart" gutterSize="none" >
            <EuiFlexItem grow={false} style={{ marginRight: 0 }}>
              <EuiIcon type={githubLogo} size="m" />
            </EuiFlexItem>
            <EuiFlexItem grow={false} style={{ marginLeft: 0 }}>
              <EuiText size="s">
                <p>
                  &nbsp;
                  <EuiLink
                    href="https://github.com/sighupio/gatekeeper-policy-manager"
                    target="_blank"
                  >
                    Source Code
                  </EuiLink>
                </p>
              </EuiText>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="l" />
    </footer>
  );
}

export default FooterComponent;
