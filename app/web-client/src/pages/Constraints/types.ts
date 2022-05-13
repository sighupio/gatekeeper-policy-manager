/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export interface IConstraintStatusPod {
  id: string;
  observedGeneration: number;
  operations: string[];
  enforced: boolean;
  constraintUID: string;
}

export interface IConstraintStatusViolation {
  enforcementAction: string;
  kind: string;
  message: string;
  name: string;
  namespace: string;
}

export interface IConstraintSpecMatchKinds {
  apiGroups: string[];
  kinds: string[];
}

export interface IConstraintSpecMatchLabelSelector {
  matchExpressions?: {
    key: string;
    operator: string;
    values: string[];
  }[];
  matchLabels?: {
    [key: string]: string;
  };
}

export interface IConstraintSpec {
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
    [key: string]: any;
  };
}

export interface IConstraint {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    creationTimestamp: string;
  };
  spec?: IConstraintSpec;
  status: {
    byPod: IConstraintStatusPod[];
    auditTimestamp: string;
    totalViolations?: number;
    violations: IConstraintStatusViolation[];
  };
}
