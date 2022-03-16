/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiButtonEmpty,
  EuiHeader,
  EuiHeaderSection,
  EuiHeaderSectionItem,
  EuiHideFor
} from "fury-design-system";
import "./Style.css";

function HeaderComponent() {
  return (
    <div className="gpm-header">
      <EuiHideFor sizes={["xs", "s"]}>
        <EuiHeader position="fixed" className="gpm-header--desktop">
          <EuiHeaderSection side="left">
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/">
                GPM
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/constrainttemplates">
                Constraint Templates
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/constraints">
                Constraints
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/configurations">
                Configurations
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
          </EuiHeaderSection>
        </EuiHeader>
      </EuiHideFor>
    </div>
  )
}

export default HeaderComponent;
