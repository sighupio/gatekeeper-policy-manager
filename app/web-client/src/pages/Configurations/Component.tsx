/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge,
  EuiButton, EuiCodeBlock, EuiDescriptionList, EuiEmptyPrompt,
  EuiFlexGroup,
  EuiFlexItem, EuiHorizontalRule, EuiIcon, EuiLink, EuiLoadingSpinner,
  EuiPage, EuiPageBody, EuiPageContent, EuiPageContentBody, EuiPageSideBar, EuiPanel, EuiSideNav,
  EuiSpacer,
  EuiText, EuiTitle, htmlIdGenerator
} from "fury-design-system";
import {useContext, useEffect, useRef, useState} from "react";
import {BackendError, ISideNav, ISideNavItem} from "../types";
import {ApplicationContext} from "../../AppContext";
import "./Style.css";
import {useLocation, useNavigate} from "react-router-dom";

interface IGVK {
  group: string;
  version: string;
  kind: string;
}

interface ITrace {
  user: string;
  kind: IGVK;
  dump: string;
}

interface IConfigSpecSync {
  syncOnly: IGVK[];
}

interface IConfigSpecValidation {
  traces: ITrace[];
}

interface IConfigSpecMatch {
  processes: string[];
  excludedNamespaces: string[];
}

interface IConfigSpecReadiness {
  statsEnabled: boolean;
}

interface IConfigSpec {
  sync?: IConfigSpecSync;
  validation?: IConfigSpecValidation;
  match?: IConfigSpecMatch[];
  readiness?: IConfigSpecReadiness;
}

interface IConfig {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp: string;
  }
  spec?: IConfigSpec;
  status?: any;
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

function generateSideNav(list: IConfig[]): ISideNav[] {
  const sideBarItems = (list ?? []).map((item, index) => {
    return {
      name: item.metadata.name,
      id: htmlIdGenerator('constraints')(),
      onClick: () => {
        scrollToElement(`#${item.metadata.name}`, true);
      },
      isSelected: index === 0,
    } as ISideNavItem;
  });

  return [{
    name: "Configurations",
    id: htmlIdGenerator('constraints')(),
    items: sideBarItems
  }]
}

function SingleConfig(item: IConfig) {
  return (
    <EuiPanel grow={true} style={{marginBottom: "24px"}}>
      <EuiFlexGroup gutterSize="s" alignItems="center">
        <EuiFlexItem>
          <EuiFlexGroup justifyContent="flexStart" style={{padding: 2}} alignItems="center">
            <EuiFlexItem grow={false}>
              <EuiText>
                <h4 style={{textTransform: "capitalize"}}>
                  {item.metadata.name}
                </h4>
              </EuiText>
            </EuiFlexItem>
            <EuiFlexItem grow={false} style={{marginLeft: "10px"}}>
              <EuiText size="xs">
                <span style={{textTransform: "uppercase", fontWeight: "bold"}}>NAMESPACE: </span> {item.metadata.namespace}
              </EuiText>
            </EuiFlexItem>
          </EuiFlexGroup>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      <EuiFlexGroup direction="column">
        <EuiFlexItem>
          <EuiAccordion
            id="accordion-1"
            buttonContent="YAML definition"
            paddingSize="l">
            <EuiCodeBlock language="json">
              {JSON.stringify(item, (k, v) => {
                if (typeof v === 'string') {
                    return v
                      .replace(/\n/g, '')
                      .replace(/"/g, "'");
                }

                return v;
              }, 2)}
            </EuiCodeBlock>
          </EuiAccordion>
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      <EuiFlexGroup direction="column">
        <EuiFlexItem>
          {!item.spec ?
          <>
            <EuiFlexGroup alignItems="center">
              <EuiFlexItem grow={false}>
                <EuiIcon type="cross" size="l" color="danger"/>
              </EuiFlexItem>
              <EuiFlexItem grow={false}>
                <EuiText size="s">
                  <h5>
                    This Configuration has no spec defined
                  </h5>
                </EuiText>
              </EuiFlexItem>
            </EuiFlexGroup>
          </>
          :
          <>
            <EuiFlexGroup alignItems="center">
              <EuiFlexItem grow={false}>
                <EuiText size="s">
                  <p style={{fontWeight: "bold"}}>
                    Spec definition for configuration
                  </p>
                </EuiText>
              </EuiFlexItem>
            </EuiFlexGroup>
          </>
          }
          {item.spec?.match &&
          <>
            <EuiSpacer />
            <EuiTitle size="xxs">
              <p>
                Match criteria
              </p>
            </EuiTitle>
            <EuiSpacer />
            <EuiFlexGroup wrap={true}>
              {item.spec.match.map(trace => {
                return (
                  <EuiFlexItem grow={false}>
                    <EuiDescriptionList
                      compressed={true}
                      type="responsiveColumn"
                      listItems={Object.entries(trace).map(k => {
                        return {
                          title: k[0],
                          description: Array.isArray(k[1]) ? k[1].join(", ") : k[1],
                        };
                      })}
                    />
                  </EuiFlexItem>
                )
              })}
            </EuiFlexGroup>
          </>
          }
          {item.spec?.readiness &&
          <EuiFlexItem>
            <EuiSpacer />
            <EuiTitle size="xxs">
              <p>
                Readiness
              </p>
            </EuiTitle>
            <EuiSpacer />
            <EuiDescriptionList
                compressed={true}
                type="responsiveColumn"
                listItems={[
                  {
                    title: "Stats Enabled",
                    description: item.spec?.readiness?.statsEnabled.toString(),
                  },
                ]}
                style={{ maxWidth: '200px' }}
            />
          </EuiFlexItem>
          }
          {item.spec?.sync &&
          <EuiFlexItem>
            <EuiSpacer />
            <EuiTitle size="xxs">
              <p>
                Sync
              </p>
            </EuiTitle>
            <EuiSpacer />
            <EuiTitle size="xxs">
              <p>
                syncOnly
              </p>
            </EuiTitle>
            <EuiSpacer />
            <EuiFlexGroup wrap={true}>
              {item.spec?.sync.syncOnly.map(sync => {
                return (
                  <EuiFlexItem grow={false}>
                    <EuiDescriptionList
                      compressed={true}
                      type="responsiveColumn"
                      listItems={Object.entries(sync).map(k => {
                        return {
                          title: k[0],
                          description: k[1],
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
          {item.spec?.validation &&
          <EuiFlexItem>
            <EuiSpacer />
            <EuiTitle size="xxs">
              <p>
                Validation
              </p>
            </EuiTitle>
            <EuiSpacer />
            <EuiTitle size="xxs">
              <p>
                traces
              </p>
            </EuiTitle>
            <EuiSpacer />
            <EuiFlexGroup wrap={true}>
              {item.spec?.validation.traces.map(trace => {
                return (
                  <EuiFlexItem grow={false}>
                    <EuiDescriptionList
                      compressed={true}
                      type="responsiveColumn"
                      listItems={Object.entries(trace).map(k => {
                        return {
                          title: k[0],
                          description: typeof k[1] !== "string" ? JSON.stringify(k[1], null, 2) : k[1],
                        };
                      })}
                    />
                  </EuiFlexItem>
                )
              })}
            </EuiFlexGroup>
          </EuiFlexItem>
          }
        </EuiFlexItem>
      </EuiFlexGroup>
      <EuiSpacer size="s"/>
      <EuiHorizontalRule margin="none"/>
      <EuiSpacer size="s"/>
      <EuiFlexGroup justifyContent="flexEnd" gutterSize="s">
        <EuiFlexItem grow={false}>
          <EuiText size="xs"
            style={{textTransform: "uppercase"}}
          >
            created on {item.metadata.creationTimestamp}
          </EuiText>
        </EuiFlexItem>
      </EuiFlexGroup>
    </EuiPanel>
  )
}

function ConfigurationsComponent() {
  const [sideNav, setSideNav] = useState<ISideNav[]>([]);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [items, setItems] = useState<IConfig[]>([]);
  const [currentElementInView, setCurrentElementInView] = useState<string>("");
  const panelsRef = useRef<HTMLDivElement[]>([]);
  const offset = 50;
  const appContextData = useContext(ApplicationContext);
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
    fetch(`${appContextData.context.apiUrl}api/v1/configs/${appContextData.context.currentK8sContext}`)
      .then(async res => {
        const body: IConfig[] = await res.json();

        if (!res.ok) {
          throw new Error(JSON.stringify(body));
        }

        setSideNav(generateSideNav(body))
        setItems(body);
      })
      .catch(err => {
        let error: BackendError
        try {
          error = JSON.parse(err.message);
        } catch (e) {
          error = {
            description: err.message,
            error: "An error occurred while fetching the configurations",
            action: "Please try again later",
          }
        }
        navigate(`/error`, {state: {error: error, entity: "configurations"}});
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
          style={{ height: "86vh" }}
          gutterSize="none"
        >
          <EuiFlexItem grow={false}>
            <EuiTitle size="l">
              <h1>Loading...</h1>
            </EuiTitle>
            <EuiSpacer size="m"/>
          </EuiFlexItem>
          <EuiFlexItem grow={false}>
            <EuiLoadingSpinner style={{width: "75px", height: "75px"}} />
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
                      return (
                        <div
                          id={`${item.metadata.name}`}
                          key={`${item.metadata.name}`}
                          ref={ref => {
                            if (ref) {
                              panelsRef.current[index] = ref;
                            }
                          }}
                        >
                          {SingleConfig(item)}
                        </div>
                      )
                    })
                    :
                    <EuiEmptyPrompt
                      iconType="alert"
                      body={
                        <p>
                          No Configuration found
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

export default ConfigurationsComponent;
