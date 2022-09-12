/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge,
  EuiBasicTable,
  EuiButton,
  EuiCallOut,
  EuiEmptyPrompt,
  EuiFlexGroup,
  EuiFlexItem,
  EuiHorizontalRule,
  EuiIcon,
  EuiLink,
  EuiLoadingSpinner,
  EuiNotificationBadge,
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
import {useCallback, useContext, useEffect, useRef, useState} from "react";
import { ApplicationContext } from "../../AppContext";
import { BackendError, ISideNav, ISideNavItem } from "../types";
import { JSONTree } from "react-json-tree";
import {useLocation, useNavigate, useParams} from "react-router-dom";
import theme from "../theme";
import { scrollToElement } from "../../utils";
import {IConstraint, IConstraintSpec} from "./types";
import useScrollToHash from "../../hooks/useScrollToHash";
import useCurrentElementInView from "../../hooks/useCurrentElementInView";
import warnIcon from "../../assets/warn.svg";
import shieldActive from "../../assets/shield-active.svg";
import shieldInactive from "../../assets/shield-inactive.svg";
import "./Style.scss";

function generateSideNav(list: IConstraint[]): ISideNav[] {
  const sideBarItems = (list ?? []).map((item, index) => {
    const enforcementRenderData = getEnforcementActionRenderData(item.spec);

    return {
      key: `${item.metadata.name}-side`,
      name: item.metadata.name,
      id: htmlIdGenerator("constraints")(),
      onClick: () => {
        scrollToElement(`#${item.metadata.name}`, true);
      },
      isSelected: index === 0,
      icon: (
        <>
          <EuiBadge
            color={item.status?.totalViolations ?? 0 > 0 ? "danger" : "success"}
          >
            {item.status.totalViolations}
          </EuiBadge>
          <EuiIcon
            type={enforcementRenderData.icon}
            title={enforcementRenderData.mode}
            style={{
              margin: "0 5px",
            }}
          />
        </>
      ),
    } as ISideNavItem;
  });

  return [
    {
      name: "Constraints",
      id: htmlIdGenerator("constraints")(),
      items: sideBarItems,
    },
  ];
}

function getEnforcementActionRenderData(spec?: IConstraintSpec) {
  let mode = "";
  let color = "hollow";
  let icon = "questionInCircle";

  if (typeof spec !== "undefined") {
    switch (spec.enforcementAction) {
      case "dryrun":
        icon = "indexRuntime";
        color = "primary";
        mode = "dryrun";
        break;
      case "warn":
        icon = warnIcon;
        color = "warning";
        mode = "warn";
        break;
      default:
        icon = "indexClose";
        color = "danger";
        mode = "deny";
        break;
    }
  }

  return {
    icon: icon,
    color: color,
    mode: mode,
    badge: (
      <EuiBadge
        color={color}
        iconType={icon}
        style={{fontSize: "10px", textTransform: "uppercase"}}
      >
        mode {mode}
      </EuiBadge>
    ),
  };
}

function SingleConstraint(item: IConstraint, context?: string) {
  return (
    <EuiPanel grow={true} style={{ marginBottom: "24px" }}>
      <EuiFlexGroup gutterSize="s" alignItems="center">
        <EuiFlexItem>
          <EuiFlexGroup
            justifyContent="flexStart"
            style={{ padding: 2 }}
            alignItems="center"
          >
            <EuiFlexItem grow={false}>
              <EuiText>
                <h4>{item.metadata.name}</h4>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              {getEnforcementActionRenderData(item.spec).badge}
            </EuiFlexItem>
            <EuiFlexItem grow={false} style={{ marginLeft: "auto" }}>
              <EuiLink href={`/constrainttemplates${context ? "/"+context : ""}#${item.kind}`}>
                <EuiText size="xs">
                  <span>TEMPLATE: {item.kind}</span>
                  <EuiIcon type="link" size="s" style={{ marginLeft: 5 }} />
                </EuiText>
              </EuiLink>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      <EuiFlexGroup direction="column" gutterSize="s">
        <EuiFlexItem grow={false}>
          {item.status?.totalViolations === undefined ? (
            <EuiFlexGroup alignItems="center" gutterSize="s">
              <EuiFlexItem grow={false}>
                <EuiIcon type="alert" size="l" color="warning" />
              </EuiFlexItem>
              <EuiFlexItem grow={false}>
                <EuiText size="s">
                  <h5>
                    Violations for this Constraint are unknown. This probably
                    means that the Constraint has not been processed by
                    Gatekeeper yet. Please, try refreshing the page.
                  </h5>
                </EuiText>
              </EuiFlexItem>
            </EuiFlexGroup>
          ) : item.status?.totalViolations === 0 ? (
            <EuiFlexGroup alignItems="center" gutterSize="s">
              <EuiFlexItem grow={false}>
                <EuiIcon type="check" size="l" color="success" />
              </EuiFlexItem>
              <EuiFlexItem grow={false}>
                <EuiText size="s">
                  <h5>There are no violations for this Constraint</h5>
                </EuiText>
              </EuiFlexItem>
            </EuiFlexGroup>
          ) : (
            <EuiFlexGroup direction="column" gutterSize="s">
              <EuiFlexItem>
                <EuiAccordion
                  id={`violations-${item.metadata.name}`}
                  buttonContent={
                    <EuiFlexGroup
                      gutterSize="s"
                      alignItems="center"
                      responsive={false}
                    >
                      <EuiFlexItem grow={false}>
                        <EuiIcon type="alert" size="m" color="danger" />
                      </EuiFlexItem>

                      <EuiFlexItem>
                        <EuiText size="xs">
                          <h4>Violations</h4>
                        </EuiText>
                      </EuiFlexItem>

                      <EuiFlexItem>
                        <EuiNotificationBadge size="s">
                          {item.status?.totalViolations}
                        </EuiNotificationBadge>
                      </EuiFlexItem>
                    </EuiFlexGroup>
                  }
                  paddingSize="none"
                >
                  <EuiFlexGroup direction="column" gutterSize="s">
                    <EuiFlexItem>
                      <EuiBasicTable
                        items={item.status.violations}
                        columns={[
                          {
                            field: "enforcementAction",
                            name: "Action",
                            truncateText: true,
                            width: "8%",
                          },
                          {
                            field: "kind",
                            name: "Kind",
                            truncateText: true,
                            width: "10%",
                          },
                          {
                            field: "namespace",
                            name: "Namespace",
                            truncateText: true,
                            width: "10%",
                          },
                          {
                            field: "name",
                            name: "Name",
                            truncateText: true,
                            width: "15%",
                          },
                          {
                            field: "message",
                            name: "Message",
                            truncateText: false,
                            width: "60%",
                          },
                        ]}
                      />
                    </EuiFlexItem>
                    <EuiFlexItem>
                      {(item.status?.totalViolations ?? 0) >
                        item.status.violations.length && (
                        <EuiCallOut
                          title="Not all violations can be shown"
                          color="warning"
                          iconType="alert"
                        >
                          <p>
                            Gatekeeper's configuration is limiting the audit
                            violations per constraint to{" "}
                            {item.status.violations.length}. See Gatekeeper's
                            --constraint-violations-limit audit configuration
                            flag.
                          </p>
                        </EuiCallOut>
                      )}
                    </EuiFlexItem>
                  </EuiFlexGroup>
                </EuiAccordion>
              </EuiFlexItem>
            </EuiFlexGroup>
          )}
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      {!item?.spec ? (
        <>
          <EuiFlexGroup alignItems="center" gutterSize="s">
            <EuiFlexItem grow={false}>
              <EuiIcon type="cross" size="l" color="danger" />
            </EuiFlexItem>
            <EuiFlexItem grow={false}>
              <EuiText size="s">
                <h5>This Constraint has no spec defined</h5>
              </EuiText>
            </EuiFlexItem>
          </EuiFlexGroup>
          <EuiSpacer size="s" />
          <EuiHorizontalRule margin="none" />
          <EuiSpacer size="s" />
        </>
      ) : (
        <>
          {item?.spec?.match && (
            <>
              <EuiFlexGroup direction="column" gutterSize="s">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{ fontWeight: "bold" }}>Match criteria</p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem>
                  <JSONTree
                    data={item?.spec?.match}
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
          )}
          {item?.spec?.parameters && (
            <>
              <EuiFlexGroup direction="column" gutterSize="s">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{ fontWeight: "bold" }}>Parameters</p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem>
                  <JSONTree
                    data={item?.spec?.parameters}
                    shouldExpandNode={() => true}
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
          )}
        </>
      )}
      <EuiFlexGroup direction="column" gutterSize="s">
        <EuiFlexItem grow={false}>
          <EuiText size="s">
            <p style={{ fontWeight: "bold" }}>
              {`Status at ${item.status.auditTimestamp}`}
            </p>
          </EuiText>
        </EuiFlexItem>
        <EuiFlexItem>
          <EuiFlexGroup direction="row" gutterSize="xs" wrap={true}>
            {item.status.byPod.map((pod) => {
              return (
                <EuiFlexItem
                  grow={false}
                  key={`${item.metadata.name}-${pod.id}`}
                >
                  <EuiBadge
                    iconType={pod.enforced ? shieldActive : shieldInactive}
                    title={`Constraint is ${
                      !pod.enforced ? "NOT " : ""
                    }being ENFORCED by this POD`}
                    style={{
                      paddingRight: 0,
                      borderRight: 0,
                      fontSize: 10,
                      position: "relative",
                    }}
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
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      <EuiFlexGroup justifyContent="flexEnd" gutterSize="s">
        <EuiFlexItem grow={false}>
          <EuiText size="xs" style={{ textTransform: "uppercase" }}>
            created on {item.metadata.creationTimestamp}
          </EuiText>
        </EuiFlexItem>
      </EuiFlexGroup>
    </EuiPanel>
  );
}

function ConstraintsComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [items, setItems] = useState<IConstraint[]>([]);
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
      `${appContextData.context.apiUrl}api/v1/constraints/${appContextData.context.currentK8sContext ?
        appContextData.context.currentK8sContext+"/" : ""}`
    )
      .then(async (res) => {
        const body: IConstraint[] = await res.json();

        if (!res.ok) {
          throw new Error(JSON.stringify(body));
        }

        setSideNav(generateSideNav(body));
        setItems(body);
      })
      .catch((err) => {
        let error: BackendError;
        try {
          error = JSON.parse(err.message);
        } catch (e) {
          error = {
            description: err.message,
            error: "An error occurred while fetching the constraints",
            action: "Please try again later",
          };
        }
        navigate(`/error`, { state: { error: error } });
      })
      .finally(() => setIsLoading(false));
  }, [appContextData.context.currentK8sContext]);

  useScrollToHash(hash, [fullyLoadedRefs, ]);

  useCurrentElementInView(panelsRef, setCurrentElementInView);

  useEffect(() => {
    if (currentElementInView) {
      const newItems = sideNav[0].items.map((item) => {
        if (item.name === currentElementInView) {
          item.isSelected = true;
        } else {
          item.isSelected = false;
        }

        return item;
      });
      setSideNav([{ ...sideNav[0], items: newItems }]);
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
            className="gpm-page gpm-page-constraints"
          >
            <EuiPageSidebar
              paddingSize="m"
              style={{
                minWidth: "300px",
              }}
              sticky
            >
              <EuiSideNav items={sideNav} />
              {items.length > 0 && (
                <EuiButton
                  iconSide="right"
                  iconSize="s"
                  iconType="popout"
                  style={{width: "100%"}}
                  href={`${appContextData.context.apiUrl}api/v1/constraints?report=html`}
                  download
                >
                  <EuiText size="xs">Download violations report</EuiText>
                </EuiButton>
              )}
            </EuiPageSidebar>
            <EuiPageBody
              paddingSize="m"
              style={{ marginBottom: 350 }}
            >
              <>
                {items && items.length > 0 ? (
                  items.map((item, index) => {
                    return (
                      <div
                        id={item.metadata.name}
                        key={item.metadata.name}
                        ref={(node) => onRefChange(node, index)}
                      >
                        {SingleConstraint(item, context)}
                      </div>
                    );
                  })
                ) : (
                  <EuiEmptyPrompt
                    iconType="alert"
                    body={<p>No Constraint found</p>}
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

export default ConstraintsComponent;
