/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiFlexGroup,
  EuiPage,
  EuiPageBody,
  EuiPageContent,
  EuiPageContentBody,
  EuiPageSideBar,
  EuiPanel,
  EuiSideNav,
  EuiSpacer,
  EuiText,
  htmlIdGenerator,
} from "fury-design-system";
import {useContext, useEffect, useState} from "react";
import {ApplicationContext} from "../../AppContext";

interface IConstraintTemplateSpecTarget {
  rego: string;
  target: string;
}

interface IConstraintTemplateSpecStatusPod {
  id: string;
  observedGeneration: number;
  operations: string[];
  templateUID: string;
}

interface IConstraintTemplate {
  apiVersion: string;
  kind: string;
  metadata: any;
  spec: {
    crd: {
      spec: {
        names: {
          kind: string;
        },
        validation: any;
      }
    },
    targets: IConstraintTemplateSpecTarget[];
  },
  status: {
    byPod: IConstraintTemplateSpecStatusPod[];
    created: boolean;
  }
}

interface IConstraintTemplateList {
  apiVersion: string;
  items: IConstraintTemplate[];
  kind: string;
  metadata: {
    continue: string;
    resourceVersion: string;
  }
}

interface ISideNavItem {
  name: string;
  id: string;
  href?: string;
  disabled?: boolean;
  isSelected?: boolean;
  onClick?: () => void;
}

interface ISideNav {
  name: string;
  id: string;
  items: ISideNavItem[];
}

function generateSideNav(list: IConstraintTemplateList): ISideNav[] {
  const sideBarItems = list.items.map(item => {
    return {
      name: item.spec.crd.spec.names.kind,
      id: htmlIdGenerator('constraint-templates')(),
    } as ISideNavItem;
  });

  return [{
    name: "Constraint Templates",
    id: htmlIdGenerator('constraint-templates')(),
    items: sideBarItems
  }]
}

function ConstraintTemplatesComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const appContextData = useContext(ApplicationContext);

  useEffect(() => {
   fetch(`${appContextData.apiUrl}api/v1/constrainttemplates`)
     .then<IConstraintTemplateList>(res => res.json())
     .then(body => {
       setSideNav(generateSideNav(body))
     })
  }, [])

  return (
    <EuiFlexGroup
      style={{minHeight: "calc(100vh - 100px)"}}
      gutterSize="none"
      direction="column"
    >
      <EuiSpacer size="xxl" />
      <EuiPage
        paddingSize="s"
        restrictWidth={1000}
        grow={true}
        className="gpm-page"
      >
        <EuiPageSideBar paddingSize="l" sticky >
          <EuiSideNav
            items={sideNav}
          />
        </EuiPageSideBar>
        <EuiPageBody>
          <EuiPageContent
            hasBorder={false}
            hasShadow={false}
            color="transparent"
            borderRadius="none"
          >
            <EuiPageContentBody restrictWidth>
              <EuiPanel grow={true}>
                <EuiText>
                  <p>I am some panel content... whose content will grow</p>
                </EuiText>
              </EuiPanel>
            </EuiPageContentBody>
          </EuiPageContent>
        </EuiPageBody>
      </EuiPage>
    </EuiFlexGroup>
  )
}

export default ConstraintTemplatesComponent;
