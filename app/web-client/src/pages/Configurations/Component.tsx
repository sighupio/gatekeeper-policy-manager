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
  EuiText
} from "fury-design-system";

function ConfigurationsComponent() {
  return (
    <EuiFlexGroup
      style={{minHeight: "calc(100vh - 100px)"}}
      gutterSize="none"
    >
      <EuiPage
        paddingSize="s"
        direction="column"
        restrictWidth={600}
        grow={true}
        className="gpm-page"
      >
        <EuiSpacer size="xxl" />
      </EuiPage>
    </EuiFlexGroup>
  )
}

export default ConfigurationsComponent;
