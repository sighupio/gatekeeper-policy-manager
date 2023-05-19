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
import { BackendError, ISideNav, ISideNavItem } from "../types";
import { ApplicationContext } from "../../AppContext";
import { useLocation, useNavigate } from "react-router-dom";
import { JSONTree } from "react-json-tree";
import theme from "../theme";
import { scrollToElement } from "../../utils";
import { IMutation } from "./types";
import useScrollToHash from "../../hooks/useScrollToHash";
import useCurrentElementInView from "../../hooks/useCurrentElementInView";
import "./Style.scss";
import clonedeep from "lodash.clonedeep";

function generateSideNav(list: IMutation[]): ISideNav[] {
  const sideBarItems = (list ?? []).map((item, index) => {
    return {
      key: `${item.metadata.name}-side`,
      name: item.metadata.name,
      id: htmlIdGenerator("constraints")(),
      onClick: () => {
        scrollToElement(`#${item.metadata.name}`, true);
      },
      isSelected: index === 0,
    } as ISideNavItem;
  });

  return [
    {
      name: "Mutations",
      id: htmlIdGenerator("mutations")(),
      items: sideBarItems,
    },
  ];
}

function SingleMutation(item: IMutation) {
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
                <h4 style={{ textTransform: "capitalize" }}>
                  {item.metadata.name}
                </h4>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem grow={false} style={{ marginLeft: "auto" }}>
              <EuiBadge
                color={"primary"}
                title={"Mutator Kind: " + item.kind}
              >
                <p>{item.kind}</p>
              </EuiBadge>
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
        </EuiFlexItem>
      </EuiFlexGroup >
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      <EuiFlexGroup direction="column">
        <EuiFlexItem>
          <EuiAccordion
            id="accordion-1"
            buttonContent="YAML definition"
            paddingSize="none"
          >
            <EuiCodeBlock
              lineNumbers
              language="json"
            >
              {JSON.stringify(
                item,
                (k, v) => {
                  if (typeof v === "string") {
                    return v.replace(/\n/g, "").replace(/"/g, "'");
                  }

                  return v;
                },
                2
              )}
            </EuiCodeBlock>
          </EuiAccordion>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      <EuiFlexGroup direction="column">
        <EuiFlexItem>
          {!item.spec ? (
            <>
              <EuiFlexGroup alignItems="center">
                <EuiFlexItem grow={false}>
                  <EuiIcon type="cross" size="l" color="danger" />
                </EuiFlexItem>
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <h5>This Mutation has no spec defined</h5>
                  </EuiText>
                </EuiFlexItem>
              </EuiFlexGroup>
            </>
          ) : (
            <>
              <EuiFlexGroup direction="column" gutterSize="s">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{ fontWeight: "bold" }}>
                      Spec definition for Mutation
                    </p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem grow={false}>
                  <JSONTree
                    data={item?.spec}
                    shouldExpandNodeInitially={() => true}
                    hideRoot={true}
                    theme={theme}
                    invertTheme={false}
                  />
                </EuiFlexItem>
              </EuiFlexGroup>
            </>
          )}
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s" />
      <EuiHorizontalRule margin="none" />
      <EuiSpacer size="s" />
      <EuiFlexGroup justifyContent="flexEnd" gutterSize="s" className="dynamic">
        <EuiFlexItem>
          <EuiFlexGroup direction="row" wrap={true} gutterSize="xs">
            {item.status.byPod.map((pod) => {
              return (
                <EuiFlexItem
                  grow={false}
                  key={`${item.metadata.name}-${pod.id}`}
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
                      style={{
                        fontSize: 10,
                        margin: "0px",
                        borderRadius: 0,
                        padding: 0,
                        verticalAlign: "baseline",
                      }}>&nbsp;</EuiBadge>
                    {pod.operations.map((operation: string) => {
                      return (
                        <EuiBadge
                          color="#ccc"
                          key={`${item.metadata.name}-${pod.id}-${operation}`}
                          style={{
                            fontSize: 10,
                            margin: "0px",
                            borderRadius: 0,
                            verticalAlign: "baseline",
                          }}> {operation}
                        </EuiBadge>
                      );
                    })}
                    <EuiBadge
                      color="#666"
                      style={{
                        fontSize: 10,
                        margin: "0px",
                        marginRight: "1px",
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

function MutationsComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [items, setItems] = useState<IMutation[]>([]);
  const [currentElementInView, setCurrentElementInView] = useState<string>("");
  const [fullyLoadedRefs, setFullyLoadedRefs] = useState<boolean>(false);
  const panelsRef = useRef<HTMLDivElement[]>([]);
  const appContextData = useContext(ApplicationContext);
  const { hash } = useLocation();
  const navigate = useNavigate();

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
      `${appContextData.context.apiUrl}api/v1/mutations/${appContextData.context.currentK8sContext ?
        appContextData.context.currentK8sContext + "/" : ""}`
    )
      .then(async (res) => {
        const body: IMutation[] = await res.json();

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
            error: "An error occurred while fetching the Mutations",
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
            className="gpm-page gpm-page-config"
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
                    return (
                      <div
                        id={`${item.metadata.name}`}
                        key={`${item.metadata.name}`}
                        ref={(node) => onRefChange(node, index)}
                      >
                        {SingleMutation(item)}
                      </div>
                    );
                  })
                ) : (
                  <EuiEmptyPrompt
                    iconType="alert"
                    body={<p>No Mutations found</p>}
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

export default MutationsComponent;
