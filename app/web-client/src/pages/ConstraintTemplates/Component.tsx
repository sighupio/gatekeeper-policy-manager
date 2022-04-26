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
  EuiHorizontalRule, EuiIcon, EuiLink,
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
import {useLocation} from "react-router-dom";
import {IConstraint} from "../Constraints/Component";

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
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp: string;
    labels: {
      [key: string]: string;
    };
    annotations: {
      [key: string]: string;
    };
  };
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

interface IRelatedConstraints {
  [key: string]: IConstraint[];
}

interface IConstraintTemplateResponse {
  constrainttemplates: IConstraintTemplateList;
  constraints_by_constrainttemplates: IRelatedConstraints;
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

function SingleConstraintTemplate(item: IConstraintTemplate, relatedConstraints: IConstraint[]) {
  return (
    <EuiPanel grow={true} style={{marginBottom: "24px"}} id={item.spec.crd.spec.names.kind}>
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
                    color={item.status?.created ? "success" : "warning"}
                    style={{textTransform: "uppercase"}}
                    title={item.status?.created ? "Created" : "Status field is not set, is Gatekeeper healthy?"}
                  >
                    <p>{item.status?.created ? "created" : "unknown state"}</p>
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
      <EuiFlexGroup direction="column" gutterSize="s">
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
          <EuiFlexGroup direction="column" gutterSize="s">
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
                  {JSON.stringify(item.spec.crd.spec?.validation?.openAPIV3Schema?.properties, null, 2)}
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
      { relatedConstraints.length > 0 &&
        <>
          <EuiFlexGroup direction="column" gutterSize="s">
            <EuiFlexItem grow={false}>
              <EuiText size="s">
                <p style={{fontWeight: "bold"}}>
                  Constraints using this template
                </p>
              </EuiText>
            </EuiFlexItem>
            {
              relatedConstraints.map((constraint, index) => (
                <EuiFlexItem key={index}>
                  <EuiLink
                    href={`/constraints#${constraint.metadata.name}`}
                  >
                    <EuiText size="s">
                      <span>{constraint.metadata.name}</span>
                      <EuiIcon type="popout" size="s" style={{marginLeft: 5}}/>
                    </EuiText>
                  </EuiLink>
                </EuiFlexItem>
              ))
            }
          </EuiFlexGroup>
          <EuiSpacer size="s"/>
          <EuiHorizontalRule margin="none"/>
          <EuiSpacer size="s"/>
        </>
      }
      <EuiFlexGroup direction="column" gutterSize="s">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{fontWeight: "bold"}}>
              Status
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          <EuiFlexGroup direction="row" wrap={true} gutterSize="xs">
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
  const [relatedConstraints, setRelatedConstraints] = useState<IRelatedConstraints>({});
  const appContextData = useContext(ApplicationContext);
  const { hash } = useLocation();

  useEffect(() => {
    fetch(`${appContextData.context.apiUrl}api/v1/constrainttemplates`)
      .then<IConstraintTemplateResponse>(res => res.json())
      .then(body => {
        setSideNav(generateSideNav(body.constrainttemplates));
        setRelatedConstraints(body.constraints_by_constrainttemplates);
        setItems(body.constrainttemplates.items);
      })
  }, [])

  useEffect(() => {
    if (hash) {
      let element = document.querySelector(hash);
      if (element) {
        element.scrollIntoView();
      }
    } else {
      window.scrollTo(0, 0);
    }
  }, [items])

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
                  const relatedConstraintsForItem = relatedConstraints[item.metadata.name] ?? [];
                  return SingleConstraintTemplate(item, relatedConstraintsForItem)
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
