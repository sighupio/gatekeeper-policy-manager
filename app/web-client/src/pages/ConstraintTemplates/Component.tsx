/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge,
  EuiCodeBlock,
  EuiEmptyPrompt,
  EuiFlexGroup,
  EuiFlexItem,
  EuiHorizontalRule,
  EuiIcon,
  EuiLink,
  EuiLoadingSpinner,
  EuiPage,
  EuiPageBody,
  EuiPageSidebar,
  EuiPanel,
  EuiSideNav,
  EuiSpacer,
  EuiText,
  EuiTitle,
  htmlIdGenerator,
} from "@elastic/eui";
import { useCallback, useContext, useEffect, useRef, useState } from "react";
import { ApplicationContext } from "../../AppContext";
import { BackendError, ISideNav, ISideNavItem } from "../types";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { IConstraint } from "../Constraints/types";
import { JSONTree } from "react-json-tree";
import theme from "../theme";
import clonedeep from "lodash.clonedeep";
import { scrollToElement } from "../../utils";
import {
  IConstraintTemplate,
  IConstraintTemplateResponse,
  IRelatedConstraints,
} from "./types";
import useScrollToHash from "../../hooks/useScrollToHash";
import useCurrentElementInView from "../../hooks/useCurrentElementInView";
import "./Style.scss";

function generateSideNav(list: IConstraintTemplate[]): ISideNav[] {
  const sideBarItems = (list ?? []).map((item, index) => {
    return {
      key: `${item.spec.crd.spec.names.kind}-side`,
      name: item.spec.crd.spec.names.kind,
      id: htmlIdGenerator("constraint-templates")(),
      onClick: () => {
        scrollToElement(`#${item.spec.crd.spec.names.kind}`, true);
      },
      isSelected: index === 0,
    } as ISideNavItem;
  });

  return [
    {
      name: "Constraint Templates",
      id: htmlIdGenerator("constraint-templates")(),
      items: sideBarItems,
    },
  ];
}

function SingleConstraintTemplate(
  item: IConstraintTemplate,
  relatedConstraints: IConstraint[],
  context?: string
) {
  return (
    <EuiPanel grow={true} style={{ marginBottom: "24px" }}>
      <EuiFlexGroup gutterSize="s" alignItems="center">
        <EuiFlexItem>
          <EuiFlexGroup
            justifyContent="spaceBetween"
            style={{ padding: 2 }}
            alignItems="flexStart"
          >
            <EuiFlexItem grow={false}>
              <EuiText>
                <h4>{item.spec.crd.spec.names.kind}</h4>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiFlexGroup direction="row" gutterSize="s" alignItems="center">
                <EuiFlexItem grow={false}>
                  <EuiBadge
                    color={item.status?.created ? "success" : "warning"}
                    style={{ textTransform: "uppercase" }}
                    title={
                      item.status?.created
                        ? "Created"
                        : "Status field is not set, is Gatekeeper healthy?"
                    }
                  >
                    <p>{item.status?.created ? "created" : "unknown state"}</p>
                  </EuiBadge>
                </EuiFlexItem>
              </EuiFlexGroup>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      {item.metadata.annotations?.description &&
        <EuiFlexGroup direction="column" gutterSize="s">
          <EuiFlexItem grow={false}>
            <EuiText size="s">
              <p>{item.metadata.annotations.description}</p>
            </EuiText>
          </EuiFlexItem>
        </EuiFlexGroup>
      }
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      <EuiFlexGroup direction="column" gutterSize="s">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{ fontWeight: "bold" }}>
              {`Target ${item.spec.targets[0].target}`}
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          {(item.spec.targets[0].libs ?? []).map((lib, index) => {
            return (
              <EuiAccordion
                id={`${item.spec.crd.spec.names.kind}-libs-${index}`}
                key={`${item.spec.crd.spec.names.kind}-libs-${index}`}
                buttonContent="Libs definition"
                paddingSize="none"
              >
                <EuiCodeBlock
                  lineNumbers
                  language="rego"
                >
                  {lib}
                </EuiCodeBlock>
              </EuiAccordion>
            )
          })}
          <EuiAccordion
            id={`${item.spec.crd.spec.names.kind}-rego`}
            buttonContent="Rego definition"
            paddingSize="none"
          >
            <EuiCodeBlock
              lineNumbers
              language="rego"
            >
              {item.spec.targets[0].rego}
            </EuiCodeBlock>
          </EuiAccordion>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      {item.spec.crd.spec?.validation?.openAPIV3Schema?.properties ? (
        <>
          <EuiFlexGroup direction="column" gutterSize="s">
            <EuiFlexItem grow={false}>
              <EuiText size="s">
                <p style={{ fontWeight: "bold" }}>Parameters schema</p>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <JSONTree
                data={
                  item.spec.crd.spec?.validation?.openAPIV3Schema?.properties
                }
                shouldExpandNodeInitially={() => true}
                hideRoot={true}
                theme={theme}
                invertTheme={false}
              />
            </EuiFlexItem>
          </EuiFlexGroup>
          <EuiSpacer size="s" />
          <EuiHorizontalRule margin="none" />
          <EuiSpacer size="s" />
        </>
      ) : (
        <></>
      )}
      {relatedConstraints.length > 0 ? (
        <>
          <EuiFlexGroup direction="column" gutterSize="s">
            <EuiFlexItem grow={false}>
              <EuiText size="s">
                <p style={{ fontWeight: "bold" }}>
                  Constraints using this template
                </p>
              </EuiText>
            </EuiFlexItem>
            {relatedConstraints.map((constraint, index) => (
              <EuiFlexItem key={constraint.metadata.name}>
                <EuiLink href={`/constraints${context ? "/" + context : ""}#${constraint.metadata.name}`}>
                  <EuiText size="s">
                    <span>{constraint.metadata.name}</span>
                    <EuiIcon type="link" size="s" style={{ marginLeft: 5 }} />
                  </EuiText>
                </EuiLink>
              </EuiFlexItem>
            ))}
          </EuiFlexGroup>
          <EuiSpacer size="s" />
          <EuiHorizontalRule margin="none" />
          <EuiSpacer size="s" />
        </>
      ) : (
        <>
          <EuiFlexGroup alignItems="center" gutterSize="s">
            <EuiFlexItem grow={false}>
              <EuiIcon type="alert" color="warning" size="l" />
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiText size="s">
                <h5>No Constraint found using this Constraint Template</h5>
              </EuiText>
            </EuiFlexItem>
          </EuiFlexGroup>
          <EuiSpacer size="s" />
          <EuiHorizontalRule margin="none" />
          <EuiSpacer size="s" />
        </>
      )}
      <EuiFlexGroup direction="column" gutterSize="s" className="dynamic">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{ fontWeight: "bold" }}>Status</p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          <EuiFlexGroup direction="row" wrap={true} gutterSize="xs">
            {item.status.byPod.map((pod) => {
              return (
                <EuiFlexItem
                  grow={false}
                  key={`${item.spec.crd.spec.names.kind}-${pod.id}`}
                >
                  <EuiBadge
                    style={{
                      paddingRight: 0,
                      borderRight: 0,
                      fontSize: 10,
                      position: "relative",
                    }}
                    className="dynamic"
                  >
                    {pod.id}
                    <EuiBadge
                      color="#666"
                      style={{
                        marginLeft: "8px",
                        borderBottomLeftRadius: 0,
                        borderTopLeftRadius: 0,
                        verticalAlign: "baseline",
                      }}
                    >
                      {`GENERATION ${pod.observedGeneration}`}
                    </EuiBadge>
                  </EuiBadge>
                </EuiFlexItem>
              );
            })}
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
    </EuiPanel>
  );
}

function ConstraintTemplatesComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [items, setItems] = useState<IConstraintTemplate[]>([]);
  const [relatedConstraints, setRelatedConstraints] =
    useState<IRelatedConstraints>({});
  const [currentElementInView, setCurrentElementInView] = useState<string>("");
  const [fullyLoadedRefs, setFullyLoadedRefs] = useState<boolean>(false);
  const panelsRef = useRef<HTMLDivElement[]>([]);
  const appContextData = useContext(ApplicationContext);
  const { hash } = useLocation();
  const navigate = useNavigate();
  const { context } = useParams<"context">();

  const onRefChange = useCallback(
    (element: HTMLDivElement | null, index: number) => {
      if (!element) {
        return;
      }

      panelsRef.current[index] = element;

      if (index === items.length - 1) {
        setFullyLoadedRefs(true);
      }
    },
    [panelsRef, items]
  );

  useEffect(() => {
    setIsLoading(true);
    fetch(
      `${appContextData.context.apiUrl}api/v1/constrainttemplates/${context ?
        context + "/" : ""}`
    )
      .then(async (res) => {
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
      .catch((err) => {
        let error: BackendError;
        try {
          error = JSON.parse(err.message);
        } catch (e) {
          error = {
            description: err.message,
            error: "An error occurred while fetching the constraint templates",
            action: "Please try again later",
          };
        }
        navigate(`/error`, {
          state: { error: error },
        });
      })
      .finally(() => setIsLoading(false));
  }, [appContextData.context.currentK8sContext]);

  useScrollToHash(hash, [fullyLoadedRefs]);

  useCurrentElementInView(panelsRef, setCurrentElementInView);

  useEffect(() => {
    if (currentElementInView) {
      const newSideBar: ISideNav[] = clonedeep(sideNav);

      newSideBar[0].items = newSideBar[0].items.map((item) => {
        if (item.name === currentElementInView) {
          item.isSelected = true;
        } else {
          item.isSelected = false;
        }

        return item;
      });

      setSideNav(newSideBar);
    }
  }, [currentElementInView]);

  return (
    <>
      {isLoading ? (
        <EuiFlexGroup
          justifyContent="center"
          alignItems="center"
          direction="column"
          style={{ height: "86vh" }}
          gutterSize="none"
        >
          <EuiFlexItem grow={false}>
            <EuiTitle size="l">
              <h1>Loading...</h1>
            </EuiTitle>
            <EuiSpacer size="m" />
          </EuiFlexItem>
          <EuiFlexItem grow={false}>
            <EuiLoadingSpinner style={{ width: "75px", height: "75px" }} />
          </EuiFlexItem>
        </EuiFlexGroup>
      ) : (
        <EuiFlexGroup
          style={{ minHeight: "calc(100vh - 100px)" }}
          gutterSize="none"
          direction="column"
        >
          <EuiPage
            paddingSize="none"
            restrictWidth={1200}
            grow={true}
            style={{ position: "relative" }}
            className="gpm-page gpm-page-constraint-templates"
          >
            <EuiPageSidebar paddingSize="m" sticky>
              <EuiSideNav items={sideNav} />
            </EuiPageSidebar>
            <EuiPageBody
              paddingSize="m"
              style={{ marginBottom: 350 }}
            >
              <>
                {items && items.length > 0 ? (
                  items.map((item, index) => {
                    const relatedConstraintsForItem =
                      relatedConstraints[item.metadata.name] ?? [];
                    return (
                      <div
                        id={item.spec.crd.spec.names.kind}
                        key={item.spec.crd.spec.names.kind}
                        ref={(node) => onRefChange(node, index)}
                      >
                        {SingleConstraintTemplate(
                          item,
                          relatedConstraintsForItem,
                          context
                        )}
                      </div>
                    );
                  })
                ) : (
                  <EuiEmptyPrompt
                    iconType="alert"
                    body={<p>No Constraint Template found</p>}
                  />
                )}
              </>
            </EuiPageBody>
          </EuiPage>
        </EuiFlexGroup>
      )}
    </>
  );
}

export default ConstraintTemplatesComponent;
