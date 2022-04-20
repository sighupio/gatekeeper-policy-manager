/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge, EuiBasicTable, EuiCallOut, EuiCodeBlock,
  EuiFlexGroup, EuiFlexItem, EuiHorizontalRule, EuiIcon, EuiNotificationBadge,
  EuiPage, EuiPageBody, EuiPageContent, EuiPageContentBody, EuiPageSideBar, EuiPanel, EuiSideNav,
  EuiSpacer, EuiTable, EuiText, EuiTitle, htmlIdGenerator,
} from "fury-design-system";
import {useContext, useEffect, useState} from "react";
import {ApplicationContext} from "../../AppContext";
import {ISideNav, ISideNavItem} from "../types";
import "./Style.css";

interface IConstraintStatusPod {
  id: string;
  observedGeneration: number;
  operations: string[];
  enforced: boolean;
  constraintUID: string;
}

interface IConstraintStatusViolation {
  enforcementAction: string;
  kind: string;
  message: string;
  name: string;
  namespace: string;
}

interface IConstraintSpec {
  enforcementAction: string;
  match: {
    kinds: {
      [key: string]: string
    }[];
  };
  parameters: {
    [key: string]: string
  };
}

interface IConstraint {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    creationTimestamp: string;
  }
  spec?: IConstraintSpec;
  status: {
    byPod: IConstraintStatusPod[];
    auditTimestamp: string;
    totalViolations?: number;
    violations: IConstraintStatusViolation[];
  }
}

function generateSideNav(list: IConstraint[]): ISideNav[] {
  const sideBarItems = list.map(item => {
    return {
      name: item.metadata.name,
      id: htmlIdGenerator('constraints')(),
    } as ISideNavItem;
  });

  return [{
    name: "Constraints",
    id: htmlIdGenerator('constraints')(),
    items: sideBarItems
  }]
}

function SingleConstraint(item: IConstraint) {
  return (
    <EuiPanel grow={true} style={{marginBottom: "24px"}}>
      <EuiFlexGroup gutterSize="s" alignItems="center">
        <EuiFlexItem>
          <EuiFlexGroup justifyContent="flexStart" style={{padding: 2}} alignItems="center">
            <EuiFlexItem grow={false}>
              <EuiText>
                <h4>
                  {item.metadata.name}
                </h4>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiBadge
                color={item.spec ? (item.spec?.enforcementAction === "dryrun" ? "primary" : "warning") : "hollow"}
                iconType={item.spec?.enforcementAction !== "dryrun" ? "lock" : "lockOpen"}
                style={{fontSize: "10px", textTransform: "uppercase"}}
              >
                mode {item.spec ? item.spec?.enforcementAction ?? "deny" : "?"}
              </EuiBadge>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiBadge
                color="hollow"
                iconType="link"
                style={{fontSize: "10px"}}
                href={`/constrainttemplates#${item.kind}`}
              >
                <span style={{textTransform: "uppercase"}}>template</span> {item.kind}
              </EuiBadge>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiBadge
                color="default"
                style={{fontSize: "10px", textTransform: "uppercase"}}
              >
                created on {item.metadata.creationTimestamp}
              </EuiBadge>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      <EuiFlexGroup direction="column">
        <EuiFlexItem grow={false}>
          {(item.status?.totalViolations ?? 0) === 0 ?
              <EuiFlexGroup alignItems="center">
                <EuiFlexItem grow={false}>
                  <EuiIcon type="check" size="l" color="success"/>
                </EuiFlexItem>
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <h5>
                      There are no violations for this Constraint
                    </h5>
                  </EuiText>
                </EuiFlexItem>
              </EuiFlexGroup> :
              <EuiFlexGroup direction="column">
                <EuiFlexItem>
                  <EuiAccordion
                    id="violations-1"
                    buttonContent={
                      <EuiFlexGroup gutterSize="s" alignItems="center" responsive={false}>
                        <EuiFlexItem grow={false}>
                          <EuiIcon type="alert" size="m" color="danger"/>
                        </EuiFlexItem>

                        <EuiFlexItem>
                          <EuiText size="xs">
                            <h4>Violations</h4>
                          </EuiText>
                        </EuiFlexItem>

                        <EuiFlexItem>
                          <EuiNotificationBadge size="s">{item.status?.totalViolations}</EuiNotificationBadge>
                        </EuiFlexItem>
                      </EuiFlexGroup>
                    }
                    paddingSize="l">
                      <EuiBasicTable
                        items={item.status.violations}
                        columns={[
                          {
                            field: "enforcementAction",
                            name: "Action",
                            truncateText: true,
                            width: "8%"
                          },
                          {
                            field: "kind",
                            name: "Kind",
                            truncateText: true,
                            width: "10%"
                          },
                          {
                            field: "namespace",
                            name: "Namespace",
                            truncateText: true,
                            width: "10%"
                          },
                          {
                            field: "name",
                            name: "Name",
                            truncateText: true,
                            width: "15%"
                          },
                          {
                            field: "message",
                            name: "Message",
                            truncateText: false,
                            width: "60%"
                          },
                        ]}
                      />
                  </EuiAccordion>
                </EuiFlexItem>
                <EuiFlexItem>
                  {(item.status?.totalViolations ?? 0) > item.status.violations.length &&
                    <EuiCallOut title="Not all violations can be shown" color="warning" iconType="alert">
                      <p>
                        Gatekeeper's configuration is limiting the audit violations per constraint to {item.status.violations.length}. See
                        Gatekeeper's --constraint-violations-limit audit configuration flag.
                      </p>
                    </EuiCallOut>
                  }
                </EuiFlexItem>
              </EuiFlexGroup>
          }
        </EuiFlexItem>
        <EuiFlexItem>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      {!item?.spec ?
        <>
          <EuiFlexGroup alignItems="center">
            <EuiFlexItem grow={false}>
              <EuiIcon type="cross" size="l" color="danger"/>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiText size="s">
                <h5>
                  This Constraint has no spec defined
                </h5>
              </EuiText>
            </EuiFlexItem>
          </EuiFlexGroup>
          <EuiSpacer size="s"/>
          <EuiHorizontalRule margin="none"/>
          <EuiSpacer size="s"/>
        </> :
        <>
          { item?.spec?.match ?
            <>
              <EuiFlexGroup direction="column">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{fontWeight: "bold"}}>
                      Match criteria
                    </p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem>
                  <EuiAccordion
                    id="accordion-3"
                    buttonContent="Schema definition"
                    paddingSize="l">
                    <EuiCodeBlock language="json">
                      {JSON.stringify(item.spec.match)}
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
          { item?.spec?.parameters ?
            <>
              <EuiFlexGroup direction="column">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{fontWeight: "bold"}}>
                      Parameters
                    </p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem>
                  <EuiAccordion
                    id="accordion-3"
                    buttonContent="Schema definition"
                    paddingSize="l">
                    <EuiCodeBlock language="json">
                      {JSON.stringify(item.spec.parameters)}
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
        </>
      }
      <EuiFlexGroup direction="column">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{fontWeight: "bold"}}>
              {`Status at ${item.status.auditTimestamp}`}
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          <EuiFlexGroup direction="row" wrap={true}>
            {item.status.byPod.map(pod => {
              return (
                <EuiFlexItem grow={false}>
                  <EuiBadge
                    iconType={pod.enforced ? "lock" : "lockOpen"}
                    title={`Constraint is ${!pod.enforced ? "NOT ": ""}being ENFORCED by this POD`}
                    style={
                      {
                        paddingRight: 0,
                        borderRight: 0,
                        fontSize: 10,
                        position: "relative"
                      }
                    }
                  >
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

function ConstraintsComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const [items, setItems] = useState<IConstraint[]>([]);
  const appContextData = useContext(ApplicationContext);

  useEffect(() => {
    fetch(`${appContextData.apiUrl}api/v1/constraints`)
      .then<IConstraint[]>(res => res.json())
      .then(body => {
        setSideNav(generateSideNav(body))
        setItems(body);
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
                  return SingleConstraint(item)
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

export default ConstraintsComponent;
