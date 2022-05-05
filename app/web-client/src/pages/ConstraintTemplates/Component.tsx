/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge,
  EuiCodeBlock, EuiEmptyPrompt,
  EuiFlexGroup,
  EuiFlexItem,
  EuiHorizontalRule, EuiIcon, EuiLink, EuiLoadingSpinner,
  EuiPage,
  EuiPageBody,
  EuiPageContent,
  EuiPageContentBody,
  EuiPageSideBar,
  EuiPanel,
  EuiSideNav,
  EuiSpacer,
  EuiText, EuiTitle,
  htmlIdGenerator,
} from "fury-design-system";
import {useContext, useEffect, useRef, useState} from "react";
import {ApplicationContext} from "../../AppContext";
import {BackendError, ISideNav, ISideNavItem} from "../types";
import {useLocation, useNavigate} from "react-router-dom";
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
  constrainttemplates: IConstraintTemplate[];
  constraints_by_constrainttemplates: IRelatedConstraints;
}

function scrollToElement(hash: string, smooth: boolean = false) {
  let element = document.querySelector(hash);

  if (!element) {
    return;
  }

  if (smooth) {
    element.scrollIntoView({behavior: 'smooth'});
  } else {
    element.scrollIntoView();
  }
}

function generateSideNav(list: IConstraintTemplate[]): ISideNav[] {
  const sideBarItems = (list ?? []).map((item, index) => {
    return {
      name: item.spec.crd.spec.names.kind,
      id: htmlIdGenerator('constraint-templates')(),
      onClick: () => {
        scrollToElement(`#${item.spec.crd.spec.names.kind}`, true);
      },
      isSelected: index === 0,
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
    <EuiPanel
      grow={true}
      style={{marginBottom: "24px"}}
    >
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
                      <EuiIcon type="link" size="s" style={{marginLeft: 5}}/>
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
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [items, setItems] = useState<IConstraintTemplate[]>([]);
  const [relatedConstraints, setRelatedConstraints] = useState<IRelatedConstraints>({});
  const [currentElementInView, setCurrentElementInView] = useState<string>("");
  const panelsRef = useRef<HTMLDivElement[]>([]);
  const appContextData = useContext(ApplicationContext);
  const offset = 50;
  const { hash } = useLocation();
  const navigate = useNavigate();

  const onScroll = () => {
    const elementVisible = panelsRef.current.filter(element => {
      const top = element.getBoundingClientRect().top;

      return top + offset >= 0 && top - offset <= window.innerHeight
    });

    if (elementVisible.length > 0) {
      setCurrentElementInView(elementVisible[0].id);
    }
  }

  useEffect(() => {
    document.addEventListener('scroll', onScroll, true)
    return () => document.removeEventListener('scroll', onScroll, true)
  }, [])

  useEffect(() => {
    setIsLoading(true);
    fetch(`${appContextData.context.apiUrl}api/v1/constrainttemplates/${appContextData.context.currentK8sContext}`)
      .then(async res => {
        const body: IConstraintTemplateResponse = await res.json();
        let constraintTemplates: IConstraintTemplate[] = [];

        if (!res.ok) {
          throw new Error(JSON.stringify(body));
        }

        if ((body?.constrainttemplates?.length ?? 0) > 0) {
          constraintTemplates = body.constrainttemplates;
        }

        setSideNav(generateSideNav(constraintTemplates));
        setRelatedConstraints(body.constraints_by_constrainttemplates);
        setItems(constraintTemplates);
      })
      .catch(err => {
        let error: BackendError
        try {
          error = JSON.parse(err.message);
        } catch (e) {
          error = {
            description: err.message,
            error: "An error occurred while fetching the constraint templates",
            action: "Please try again later",
          }
        }
        navigate(`/error`, {state: {error: error, entity: "constrainttemplates"}});
      })
      .finally(() => setIsLoading(false));
  }, [appContextData.context.currentK8sContext])

  useEffect(() => {
    if (hash) {
      scrollToElement(hash);
    } else {
      window.scrollTo(0, 0);
    }
  }, [items])

  useEffect(() => {
    if (currentElementInView) {
      const newItems = sideNav[0].items.map(item => {
        if (item.name === currentElementInView) {
          item.isSelected = true;
        } else {
          item.isSelected = false;
        }

        return item;
      })
      setSideNav([{ ...sideNav[0], items: newItems }]);
    }
  }, [currentElementInView])

  return (
    <>
      {
        isLoading ?
          <EuiFlexGroup
            justifyContent="center"
            alignItems="center"
            direction="column"
            style={{height: "86vh"}}
            gutterSize="none"
          >
            <EuiFlexItem grow={false}>
              <EuiTitle size="l">
                <h1>Loading...</h1>
              </EuiTitle>
              <EuiSpacer size="m"/>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiLoadingSpinner style={{width: "75px", height: "75px"}}/>
            </EuiFlexItem>
          </EuiFlexGroup> :
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
                  <EuiPageContentBody
                    restrictWidth
                    style={{marginBottom: 350}}
                  >
                    {items && items.length > 0 ?
                      items.map((item, index) => {
                        const relatedConstraintsForItem = relatedConstraints[item.metadata.name] ?? [];
                        return (
                          <div
                            id={item.spec.crd.spec.names.kind}
                            key={item.spec.crd.spec.names.kind}
                            ref={ref => {
                              if (ref) {
                                panelsRef.current[index] = ref;
                              }
                            }}
                          >
                            {SingleConstraintTemplate(item, relatedConstraintsForItem)}
                          </div>
                        );
                      })
                      :
                      <EuiEmptyPrompt
                        iconType="alert"
                        body={
                          <p>
                            No Constraint Template found
                          </p>
                        }
                      />
                    }
                  </EuiPageContentBody>
                </EuiPageContent>
              </EuiPageBody>
            </EuiPage>
          </EuiFlexGroup>
      }
    </>
  )
}

export default ConstraintTemplatesComponent;
