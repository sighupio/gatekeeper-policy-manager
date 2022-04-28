/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge, EuiBasicTable, EuiButton, EuiCallOut, EuiCodeBlock, EuiDescriptionList,
  EuiFlexGroup, EuiFlexItem, EuiHorizontalRule, EuiIcon, EuiLink, EuiListGroup, EuiListGroupItem, EuiNotificationBadge,
  EuiPage, EuiPageBody, EuiPageContent, EuiPageContentBody, EuiPageSideBar, EuiPanel, EuiSideNav,
  EuiSpacer, EuiTable, EuiText, EuiTitle, htmlIdGenerator,
} from "fury-design-system";
import {useContext, useEffect, useState} from "react";
import {ApplicationContext} from "../../AppContext";
import {ISideNav, ISideNavItem} from "../types";
import "./Style.css";
import {useLocation} from "react-router-dom";

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

interface IConstraintSpecMatchKinds {
  apiGroups: string[];
  kinds: string[];
}

interface IConstraintSpecMatchLabelSelector {
  matchExpressions?: {
    key: string;
    operator: string;
    values: string[];
  }[];
  matchLabels?: {
    [key: string]: string;
  };
}


interface IConstraintSpec {
  enforcementAction: string;
  match?: {
    kinds?: IConstraintSpecMatchKinds[];
    scope?: string;
    namespaces?: string[];
    excludedNamespaces?: string[];
    labelSelector?: IConstraintSpecMatchLabelSelector;
    namespaceSelector?: IConstraintSpecMatchLabelSelector;
    name?: string;
  };
  parameters: {
    [key: string]: any
  };
}

export interface IConstraint {
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
    <EuiPanel grow={true} style={{marginBottom: "24px"}} id={item.metadata.name}>
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
            <EuiFlexItem grow={false} style={{marginLeft: "auto"}}>
              <EuiLink
                href={`/constrainttemplates#${item.kind}`}
              >
                <EuiText size="xs">
                  <span>TEMPLATE: {item.kind}</span>
                  <EuiIcon type="link" size="s" style={{marginLeft: 5}}/>
                </EuiText>
              </EuiLink>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      <EuiFlexGroup direction="column" gutterSize="s">
        <EuiFlexItem grow={false}>
          {item.status?.totalViolations === undefined ?
            <EuiFlexGroup alignItems="center" gutterSize="s">
              <EuiFlexItem grow={false}>
                <EuiIcon type="alert" size="l" color="warning" />
              </EuiFlexItem>
              <EuiFlexItem grow={false}>
                <EuiText size="s">
                  <h5>
                    Violations for this Constraint are unknown. This probably means that the Constraint has not been processed by Gatekeeper yet. Please, try refreshing the page.
                  </h5>
                </EuiText>
              </EuiFlexItem>
            </EuiFlexGroup>
          : item.status?.totalViolations === 0 ?
              <EuiFlexGroup alignItems="center" gutterSize="s">
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
              <EuiFlexGroup direction="column" gutterSize="s">
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
                      <EuiFlexGroup direction="column" gutterSize="s">
                        <EuiFlexItem>
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
                  </EuiAccordion>
                </EuiFlexItem>
              </EuiFlexGroup>
          }
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      {!item?.spec ?
        <>
          <EuiFlexGroup alignItems="center" gutterSize="s">
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
          {
            item?.spec?.match &&
            <>
              <EuiFlexGroup direction="column" gutterSize="s">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{fontWeight: "bold"}}>
                      Match criteria
                    </p>
                  </EuiText>
                </EuiFlexItem>
                {
                  item?.spec?.match?.kinds &&
                  <EuiFlexItem>
                    <EuiSpacer />
                    <EuiTitle size="xxs">
                      <p>
                        Kinds
                      </p>
                    </EuiTitle>
                    <EuiSpacer />
                    <EuiFlexGroup wrap={true}>
                    {item?.spec?.match?.kinds.map(kind => {
                      return (
                        <EuiFlexItem grow={false}>
                          <EuiDescriptionList
                            compressed={true}
                            type="responsiveColumn"
                            listItems={Object.entries(kind).map(k => {
                              return {
                                title: k[0],
                                description: k[1].length > 0 ? k[1][0] === '' ? "empty (core)" : k[1].join(", ") : "empty (core)",
                              };
                            })}
                            style={{ maxWidth: '200px' }}
                          />
                        </EuiFlexItem>
                      )
                    })}
                    </EuiFlexGroup>
                  </EuiFlexItem>
                }
                {
                  item?.spec?.match?.scope &&
                  <EuiFlexItem>
                    <EuiSpacer />
                    <EuiTitle size="xxs">
                      <p>
                        Scope
                      </p>
                    </EuiTitle>
                    <EuiSpacer />
                    <EuiText size="s">
                      <p>
                        {item?.spec?.match?.scope}
                      </p>
                    </EuiText>
                  </EuiFlexItem>
                }
                {
                  item?.spec?.match?.name &&
                  <EuiFlexItem>
                    <EuiSpacer />
                    <EuiTitle size="xxs">
                      <p>
                        Name
                      </p>
                    </EuiTitle>
                    <EuiSpacer />
                    <EuiText size="s">
                      <p>
                        {item?.spec?.match?.name}
                      </p>
                    </EuiText>
                  </EuiFlexItem>
                }
                {
                  item?.spec?.match?.namespaces &&
                  <EuiFlexItem>
                    <EuiSpacer />
                    <EuiTitle size="xxs">
                      <p>
                        Namespaces
                      </p>
                    </EuiTitle>
                    <EuiSpacer />
                    <EuiText size="s">
                      <p>
                        {item?.spec?.match?.namespaces.join(", ")}
                      </p>
                    </EuiText>
                  </EuiFlexItem>
                }
                {
                  item?.spec?.match?.excludedNamespaces &&
                  <EuiFlexItem>
                    <EuiSpacer />
                    <EuiTitle size="xxs">
                      <p>
                        Excluded Namespaces
                      </p>
                    </EuiTitle>
                    <EuiSpacer />
                    <EuiText size="s">
                      <p>
                        {item?.spec?.match?.excludedNamespaces.join(", ")}
                      </p>
                    </EuiText>
                  </EuiFlexItem>
                }
                {
                  item?.spec?.match?.labelSelector &&
                  <EuiFlexItem>
                    <EuiSpacer />
                    <EuiTitle size="xxs">
                      <p>
                        Label Selector
                      </p>
                    </EuiTitle>
                    <EuiSpacer />
                    {
                      item?.spec?.match?.labelSelector?.matchExpressions &&
                      <>
                        <EuiTitle size="xxs">
                          <p>
                            Match Expressions
                          </p>
                        </EuiTitle>
                        <EuiSpacer />
                        <EuiFlexGroup wrap={true}>
                          {item?.spec?.match?.labelSelector.matchExpressions.map(expr => {
                            return (
                              <EuiFlexItem grow={false}>
                                <EuiDescriptionList
                                  compressed={true}
                                  type="responsiveColumn"
                                  listItems={Object.entries(expr).map(k => {
                                    return {
                                      title: k[0],
                                      description: Array.isArray(k[1]) ? k[1].join(", ") : k[1],
                                    };
                                  })}
                                  style={{ maxWidth: '200px' }}
                                />
                              </EuiFlexItem>
                            )
                          })}
                        </EuiFlexGroup>
                      </>
                    }
                    {
                      item?.spec?.match?.labelSelector?.matchLabels &&
                      <>
                        <EuiSpacer />
                        <EuiTitle size="xxs">
                          <p>
                            Match Labels
                          </p>
                        </EuiTitle>
                        <EuiSpacer />
                        <EuiDescriptionList
                          compressed={true}
                          type="responsiveColumn"
                          listItems={Object.entries(item?.spec?.match?.labelSelector?.matchLabels).map(k => {
                            return {
                              title: k[0],
                              description: k[1],
                            };
                          })}
                          style={{ maxWidth: '200px' }}
                        />
                      </>
                    }
                  </EuiFlexItem>
                }
                {
                  item?.spec?.match?.namespaceSelector &&
                    <EuiFlexItem>
                      <EuiSpacer />
                      <EuiTitle size="xxs">
                        <p>
                          Namespace Selector
                        </p>
                      </EuiTitle>
                      <EuiSpacer />
                      {
                        item?.spec?.match?.namespaceSelector?.matchExpressions &&
                          <>
                            <EuiTitle size="xxs">
                              <p>
                                Match Expressions
                              </p>
                            </EuiTitle>
                            <EuiSpacer />
                            <EuiFlexGroup wrap={true}>
                              {item?.spec?.match?.namespaceSelector.matchExpressions.map(expr => {
                                return (
                                  <EuiFlexItem grow={false}>
                                    <EuiDescriptionList
                                      compressed={true}
                                      type="responsiveColumn"
                                      listItems={Object.entries(expr).map(k => {
                                        return {
                                          title: k[0],
                                          description: Array.isArray(k[1]) ? k[1].join(", ") : k[1],
                                        };
                                      })}
                                      style={{ maxWidth: '200px' }}
                                    />
                                  </EuiFlexItem>
                                )
                              })}
                            </EuiFlexGroup>
                          </>
                      }
                      {
                        item?.spec?.match?.namespaceSelector?.matchLabels &&
                          <>
                            <EuiSpacer />
                            <EuiTitle size="xxs">
                              <p>
                                Match Labels
                              </p>
                            </EuiTitle>
                            <EuiSpacer />
                            <EuiDescriptionList
                              compressed={true}
                              type="responsiveColumn"
                              listItems={Object.entries(item?.spec?.match?.namespaceSelector?.matchLabels).map(k => {
                                return {
                                  title: k[0],
                                  description: k[1],
                                };
                              })}
                              style={{ maxWidth: '200px' }}
                            />
                          </>
                      }
                    </EuiFlexItem>
                }
              </EuiFlexGroup>
              <EuiSpacer size="s"/>
              <EuiHorizontalRule margin="none"/>
              <EuiSpacer size="s"/>
            </>
          }
          { item?.spec?.parameters &&
            <>
              <EuiFlexGroup direction="column" gutterSize="s">
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
                      {JSON.stringify(item.spec.parameters, null, 2)}
                    </EuiCodeBlock>
                  </EuiAccordion>
                </EuiFlexItem>
              </EuiFlexGroup>
              <EuiSpacer size="s"/>
              <EuiHorizontalRule margin="none"/>
              <EuiSpacer size="s"/>
            </>
          }
        </>
      }
      <EuiFlexGroup direction="column" gutterSize="s">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{fontWeight: "bold"}}>
              {`Status at ${item.status.auditTimestamp}`}
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          <EuiFlexGroup direction="row" gutterSize="xs" wrap={true}>
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
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      <EuiFlexGroup justifyContent="flexEnd" gutterSize="s">
        <EuiFlexItem grow={false}>
          <EuiText size="xs" style={{textTransform: "uppercase"}}>
            created on {item.metadata.creationTimestamp}
          </EuiText>
        </EuiFlexItem>
      </EuiFlexGroup>
    </EuiPanel>
  )
}

function ConstraintsComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const [items, setItems] = useState<IConstraint[]>([]);
  const appContextData = useContext(ApplicationContext);
  const { hash } = useLocation();

  useEffect(() => {
    fetch(`${appContextData.context.apiUrl}api/v1/constraints/${appContextData.context.currentK8sContext}`)
      .then<IConstraint[]>(res => res.json())
      .then(body => {
        setSideNav(generateSideNav(body))
        setItems(body);
      })
      .catch(err => {
        setItems([]);
        console.error(err);
      });
  }, [appContextData.context.currentK8sContext])

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
        <EuiPageSideBar
          paddingSize="l"
          style={{
            minWidth: "270px",
          }}
          sticky
        >
          <EuiSideNav
            items={sideNav}
          />
          <EuiButton
            iconSide="right"
            iconSize="s"
            iconType="popout"
            href={`${appContextData.context.apiUrl}api/v1/constraints?report=html`}
            download
          >
            <EuiText size="xs">
              Download violations report
            </EuiText>
          </EuiButton>
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
