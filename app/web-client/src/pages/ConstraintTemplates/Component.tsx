/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge,
  EuiCodeBlock,
  EuiFlexGroup,
  EuiFlexItem,
  EuiHorizontalRule,
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
import {ISideNav, ISideNavItem} from "../types";

interface IConstraintTemplateSpecTarget {
  rego: string;
  libs?: string;
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

function SingleConstraintTemplate(item: IConstraintTemplate) {
  return (
    <EuiPanel grow={true} style={{marginBottom: "24px"}}>
      <EuiFlexGroup gutterSize="s" alignItems="center">
        <EuiFlexItem>
          <EuiFlexGroup justifyContent="spaceBetween" style={{padding: 2}} alignItems="flexStart">
            <EuiFlexItem grow={false}>
              <EuiText>
                <h4>
                  {item.spec.crd.spec.names.kind}
                </h4>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiFlexGroup direction="row" gutterSize="s" alignItems="center">
                <EuiFlexItem grow={false}>
                  <EuiBadge
                    color="success"
                    style={{textTransform: "uppercase"}}
                  >
                    <p>created</p>
                  </EuiBadge>
                </EuiFlexItem>
              </EuiFlexGroup>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      <EuiFlexGroup direction="column">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{fontWeight: "bold"}}>
              {`Target ${item.spec.targets[0].target}`}
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          {item.spec.targets[0].libs
            ?
            <>
              <EuiAccordion
                id="accordion-1"
                buttonContent="Libs definition"
                paddingSize="l">
                <EuiCodeBlock language="rego">
                  {item.spec.targets[0].libs}
                </EuiCodeBlock>
              </EuiAccordion>
              <EuiSpacer size="s"/>
            </>
            :
            <></>
          }
          <EuiAccordion
            id="accordion-2"
            buttonContent="Rego definition"
            paddingSize="l">
            <EuiCodeBlock language="rego">
              {item.spec.targets[0].rego}
            </EuiCodeBlock>
          </EuiAccordion>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      {item.spec.crd.spec?.validation?.openAPIV3Schema?.properties ?
        <>
          <EuiFlexGroup direction="column">
            <EuiFlexItem grow={false}>
              <EuiText size="s">
                <p style={{fontWeight: "bold"}}>
                  {`Parameters schema`}
                </p>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem>
              <EuiAccordion
                id="accordion-3"
                buttonContent="Schema definition"
                paddingSize="l">
                <EuiCodeBlock language="json">
                  {JSON.stringify(item.spec.crd.spec?.validation?.openAPIV3Schema?.properties)}
                </EuiCodeBlock>
              </EuiAccordion>
            </EuiFlexItem>
          </EuiFlexGroup>
          <EuiSpacer size="s"/>
          <EuiHorizontalRule margin="none"/>
          <EuiSpacer size="s"/>
        </> :
        <></>
      }
      <EuiFlexGroup direction="column">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{fontWeight: "bold"}}>
              {`Status`}
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          <EuiFlexGroup direction="row" wrap={true}>
            {item.status.byPod.map(pod => {
              return (
                <EuiFlexItem grow={false}>
                  <EuiBadge style={
                    {
                      paddingRight: 0,
                      borderRight: 0,
                      fontSize: 10,
                      position: "relative"
                    }
                  }>
                    {pod.id}
                    <EuiBadge color="#666" style={
                      {
                        marginLeft: "8px",
                        borderBottomLeftRadius: 0,
                        borderTopLeftRadius: 0,
                        verticalAlign: "baseline"
                      }
                    }>
                      {`GENERATION ${pod.observedGeneration}`}
                    </EuiBadge>
                  </EuiBadge>
                </EuiFlexItem>
              )
            })}
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
    </EuiPanel>
  )
}

function ConstraintTemplatesComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const [items, setItems] = useState<IConstraintTemplate[]>([]);
  const appContextData = useContext(ApplicationContext);

  useEffect(() => {
    fetch(`${appContextData.apiUrl}api/v1/constrainttemplates`)
      .then<IConstraintTemplateList>(res => res.json())
      .then(body => {
        setSideNav(generateSideNav(body))
        setItems(body.items);
      })
  }, [])

  return (
    <EuiFlexGroup
      style={{minHeight: "calc(100vh - 100px)"}}
      gutterSize="none"
      direction="column"
    >
      <EuiPage
        paddingSize="none"
        restrictWidth={1100}
        grow={true}
        style={{position: "relative"}}
        className="gpm-page"
      >
        <EuiPageSideBar paddingSize="l" sticky>
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
              {items && items.length > 0 ?
                items.map(item => {
                  return SingleConstraintTemplate(item)
                })
                :
                <></>
              }
            </EuiPageContentBody>
          </EuiPageContent>
        </EuiPageBody>
      </EuiPage>
    </EuiFlexGroup>
  )
}

export default ConstraintTemplatesComponent;
