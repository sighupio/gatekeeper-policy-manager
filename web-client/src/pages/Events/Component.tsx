/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiBasicTable,
  EuiBasicTableColumn,
  EuiLink,
  EuiIcon,
  EuiButtonIcon,
  EuiEmptyPrompt,
  EuiFlexGroup,
  EuiFlexItem,
  EuiLoadingSpinner,
  EuiPage,
  EuiPageBody,
  EuiPanel,
  EuiSpacer,
  EuiTitle,
  EuiScreenReaderOnly,
  EuiText,
} from "@elastic/eui";
import { useContext, useEffect, useState, ReactNode } from "react";
import { ApplicationContext } from "../../AppContext";
import { BackendError } from "../types";
import { useNavigate } from "react-router-dom";
import { IEvent } from "./types";


function EventsComponent() {

  const [isLoading, setIsLoading] = useState<boolean>(true);
  const [items, setItems] = useState<IEvent[]>([]);
  const appContextData = useContext(ApplicationContext);
  const navigate = useNavigate();

  const columns: Array<EuiBasicTableColumn<IEvent>> = [
    {
      field: "firstTimestamp",
      name: "First Timestamp",
      dataType: "date",

    },
    {
      field: "lastTimestamp",
      name: "Last Timestamp",
      dataType: "date",
    },
    {
      field: "count",
      name: "Count",
      align: "center",
    },
    {
      field: "reason",
      name: "Reason",
    },
    {
      field: "metadata.annotations.constraint_action",
      name: "Action",
    },
    {
      // field: "metadata.annotations.constraint_kind",
      name: "Template",

      render: (e: IEvent) => (<EuiLink href={`/constrainttemplates${appContextData.context.currentK8sContext ? "/" + appContextData.context.currentK8sContext : ""}#${e.metadata.annotations.constraint_kind}`}>
        <EuiText size="xs">
          <span>{e.metadata.annotations.constraint_kind}</span>
          <EuiIcon type="link" style={{ marginLeft: 5 }} />
        </EuiText>
      </EuiLink>)
    },
    {
      // field: "metadata.annotations.constraint_name",
      name: "Constraint",

      render: (e: IEvent) => (<EuiLink href={`/constraints${appContextData.context.currentK8sContext ? "/" + appContextData.context.currentK8sContext : ""}#${e.metadata.annotations.constraint_name}`}>
        <EuiText size="xs">
          <span>{e.metadata.annotations.constraint_name}</span>
          <EuiIcon type="link" style={{ marginLeft: 5 }} />
        </EuiText>
      </EuiLink>)
    },
  ]

  // exapand
  const [itemIdToExpandedRowMap, setItemIdToExpandedRowMap] = useState<
    Record<string, ReactNode>
  >({});

  const toggleDetails = (k8sevent: IEvent) => {
    const itemIdToExpandedRowMapValues = { ...itemIdToExpandedRowMap };

    if (itemIdToExpandedRowMapValues[k8sevent.metadata.name]) {
      delete itemIdToExpandedRowMapValues[k8sevent.metadata.name];
    } else {
      itemIdToExpandedRowMapValues[k8sevent.metadata.name] = (
        <EuiFlexGroup direction="column">
          <EuiFlexItem>
            <EuiFlexGroup direction="column" gutterSize="xs">
              <EuiText size="s">
                <h6>Event Details</h6>
              </EuiText>
              <EuiFlexGroup direction="row">
                <EuiFlexItem grow={false}><strong>Type</strong>{k8sevent.metadata.annotations.event_type}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Process</strong>{k8sevent.metadata.annotations.process}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Request Username</strong>{k8sevent.metadata.annotations.request_username}</EuiFlexItem>
              </EuiFlexGroup>
            </EuiFlexGroup>
          </EuiFlexItem>
          <EuiFlexItem>
            <EuiFlexGroup direction="column" gutterSize="xs">
              <EuiText size="s">
                <h6>Resource Details</h6>
              </EuiText>
              <EuiFlexGroup direction="row">
                <EuiFlexItem grow={false}><strong>API Version</strong>{k8sevent.metadata.annotations.resource_api_version}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Group</strong>{k8sevent.metadata.annotations.resource_group}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Kind</strong>{k8sevent.metadata.annotations.resource_kind}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Name</strong>{k8sevent.metadata.annotations.resource_name}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Namespace</strong>{k8sevent.metadata.annotations.resource_namespace}</EuiFlexItem>
              </EuiFlexGroup>
            </EuiFlexGroup>
          </EuiFlexItem>
          <EuiFlexItem>
            <EuiFlexGroup direction="column" gutterSize="xs">
              <EuiText size="s">
                <h6>Involved Object</h6>
              </EuiText>
              <EuiFlexGroup direction="row">
                <EuiFlexItem grow={false}><strong>Namespace</strong>{k8sevent.involvedObject.namespace}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Kind</strong>{k8sevent.involvedObject.kind}</EuiFlexItem>
                <EuiFlexItem grow={false}><strong>Name</strong>{k8sevent.involvedObject.name}</EuiFlexItem>
              </EuiFlexGroup>
            </EuiFlexGroup>
          </EuiFlexItem>
          <EuiFlexItem>
            <EuiFlexGroup direction="column" gutterSize="xs">
              <EuiText size="s">
                <h6> Message</h6>
              </EuiText>
              <p>{k8sevent.message}</p>
            </EuiFlexGroup>
          </EuiFlexItem>
        </EuiFlexGroup>
      );
    }
    setItemIdToExpandedRowMap(itemIdToExpandedRowMapValues);

  };

  const columnsWithExpandingRowToggle: Array<EuiBasicTableColumn<IEvent>> = [
    ...columns,
    {
      align: 'right',
      width: '40px',
      isExpander: true,
      name: (
        <EuiScreenReaderOnly>
          <span>Expand rows</span>
        </EuiScreenReaderOnly>
      ),
      render: (k8sevent: IEvent) => {
        const itemIdToExpandedRowMapValues = { ...itemIdToExpandedRowMap };

        return (
          <EuiButtonIcon
            onClick={() => toggleDetails(k8sevent)}
            aria-label={
              itemIdToExpandedRowMapValues[k8sevent.metadata.name] ? 'Collapse' : 'Expand'
            }
            iconType={
              itemIdToExpandedRowMapValues[k8sevent.metadata.name] ? 'arrowDown' : 'arrowRight'
            }
          />
        );
      },
    },
  ];

  useEffect(() => {
    setIsLoading(true);
    fetch(
      `${appContextData.context.apiUrl}api/v1/events/${appContextData.context.currentK8sContext ?
        appContextData.context.currentK8sContext + "/" : ""}`
    )
      .then(async (res) => {
        const body: IEvent[] = await res.json();

        if (!res.ok) {
          throw new Error(JSON.stringify(body));
        }
        setItems(body);
      })
      .catch((err) => {
        let error: BackendError;
        try {
          error = JSON.parse(err.message);
        } catch (e) {
          error = {
            description: err.message,
            error: "An error occurred while fetching the events",
            action: "Please try again later",
          };
        }
        navigate(`/error`, { state: { error: error } });
      })
      .finally(() => setIsLoading(false));
  }, [appContextData.context.currentK8sContext]);

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
            restrictWidth={1440}
            grow={true}
            style={{ position: "relative" }}
            className="gpm-page"
          >
            <EuiPageBody
              paddingSize="m"
              style={{ marginBottom: 350 }}
            >
              <>
                {items && items.length > 0 ? (
                  <EuiPanel>
                    <EuiBasicTable
                      tableCaption="Table of Kubernetes Events generated by Gatekeeper"
                      tableLayout="auto"
                      // compressed={true}
                      itemId={item => item.metadata.name}
                      itemIdToExpandedRowMap={itemIdToExpandedRowMap}
                      isExpandable={true}
                      items={items}
                      columns={columnsWithExpandingRowToggle}
                    // sorting={}
                    />
                  </EuiPanel>
                ) : (
                  <EuiEmptyPrompt
                    iconType="alert"
                    body={<p>No Events found.<br />Gatekeeper emiting events on violations is an <a href="https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup/#alpha-emit-admission-and-audit-events">alpha feature</a>. Make sure that it is enabled.</p>}
                  />
                )}
              </>
            </EuiPageBody>
          </EuiPage>
        </EuiFlexGroup >
      )
      }
    </>
  );
}

export default EventsComponent;
