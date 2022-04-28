/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiAccordion,
  EuiBadge,
  EuiButton, EuiCodeBlock,
  EuiFlexGroup,
  EuiFlexItem, EuiHorizontalRule, EuiIcon, EuiLink,
  EuiPage, EuiPageBody, EuiPageContent, EuiPageContentBody, EuiPageSideBar, EuiPanel, EuiSideNav,
  EuiSpacer,
  EuiText, htmlIdGenerator
} from "fury-design-system";
import {useContext, useEffect, useState} from "react";
import {ISideNav, ISideNavItem} from "../types";
import {ApplicationContext} from "../../AppContext";
import {json} from "stream/consumers";

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

function generateSideNav(list: IConfig[]): ISideNav[] {
  const sideBarItems = list.map(item => {
    return {
      name: item.metadata.name,
      id: htmlIdGenerator('constraints')(),
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
            <EuiSpacer size="s"/>
            <EuiSpacer size="s"/>
          </>
          }
          {item.spec?.match &&
          <>
            <EuiFlexGroup direction="column" gutterSize="s">
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
                    {JSON.stringify(item.spec.match, null, 2)}
                  </EuiCodeBlock>
                </EuiAccordion>
              </EuiFlexItem>
            </EuiFlexGroup>
            <EuiSpacer size="s"/>
            <EuiSpacer size="s"/>
          </>
          }
          {item.spec?.readiness &&
            <>
              <EuiFlexGroup>
                <EuiFlexItem>
                  <EuiText size="s">
                    <p style={{fontWeight: "bold"}}>
                      Readiness
                    </p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem>
                  <EuiText size="s">
                    <p>
                      Stats Enabled: {item.spec?.readiness?.statsEnabled}
                    </p>
                  </EuiText>
                </EuiFlexItem>
              </EuiFlexGroup>
              <EuiSpacer size="s"/>
              <EuiSpacer size="s"/>
            </>
          }
          {item.spec?.sync &&
            <>
              <EuiFlexGroup direction="column" gutterSize="s">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{fontWeight: "bold"}}>
                      Sync
                    </p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem>
                  <EuiAccordion
                    id="accordion-3"
                    buttonContent="Schema definition"
                    paddingSize="l">
                    <EuiCodeBlock language="json">
                      {JSON.stringify(item.spec.sync, null, 2)}
                    </EuiCodeBlock>
                  </EuiAccordion>
                </EuiFlexItem>
              </EuiFlexGroup>
              <EuiSpacer size="s"/>
              <EuiSpacer size="s"/>
            </>
          }
          {item.spec?.validation &&
            <>
              <EuiFlexGroup direction="column" gutterSize="s">
                <EuiFlexItem grow={false}>
                  <EuiText size="s">
                    <p style={{fontWeight: "bold"}}>
                      Validation
                    </p>
                  </EuiText>
                </EuiFlexItem>
                <EuiFlexItem>
                  <EuiAccordion
                    id="accordion-3"
                    buttonContent="Schema definition"
                    paddingSize="l">
                    <EuiCodeBlock language="json">
                      {JSON.stringify(item.spec.validation, null, 2)}
                    </EuiCodeBlock>
                  </EuiAccordion>
                </EuiFlexItem>
              </EuiFlexGroup>
            </>
          }
        </EuiFlexItem>
      </EuiFlexGroup>
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
  const [items, setItems] = useState<IConfig[]>([]);
  const appContextData = useContext(ApplicationContext);

  useEffect(() => {
    fetch(`${appContextData.context.apiUrl}api/v1/configs/${appContextData.context.currentK8sContext}`)
      .then<IConfig[]>(res => res.json())
      .then(body => {
        setSideNav(generateSideNav(body))
        setItems(body);
      })
      .catch(err => {
        setItems([]);
        console.error(err);
      });
  }, [appContextData.context.currentK8sContext])

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
                  return SingleConfig(item)
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

export default ConfigurationsComponent;
