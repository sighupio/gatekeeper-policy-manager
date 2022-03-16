/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiFlexGroup,
  EuiFlexItem, EuiIcon, EuiLink, EuiText,
} from "fury-design-system";
import githubLogo from '../../assets/github-logo.svg';
import "./Style.css";

function FooterComponent() {
  return (
    <footer className="gpm-footer">
      <EuiFlexGroup justifyContent="spaceBetween">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p>
              <strong>Gatekeeper Policy Manager v0.5.0</strong>
            </p>
          </EuiText>
          <EuiText size="s">
            <p>
              A simple to use web-based Gatekeeper policies manager
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p>
              <EuiIcon type="heart" color="red" />
              {' '} Proud part of the {' '}
              <EuiLink href="https://kubernetesfury.com/" target="_blank">
                Kubernetes Fury Distribution
              </EuiLink>
            </p>
          </EuiText>
          <EuiFlexGroup justifyContent="flexStart">
            <EuiFlexItem grow={false} style={{"marginRight": 0}}>
              <EuiIcon type={githubLogo} size="m"/>
            </EuiFlexItem>
            <EuiFlexItem grow={false} style={{"marginLeft": 0}}>
              <EuiText size="s">
                <p>
                  &nbsp;
                  <EuiLink href="https://github.com/sighupio/gatekeeper-policy-manager" target="_blank">
                    Source Code
                  </EuiLink>
                </p>
              </EuiText>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
    </footer>
  )
}

export default FooterComponent;
