import { IConstraint } from "../Constraints/types";

/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export interface IConstraintTemplateSpecTarget {
  rego: string;
  libs?: string[];
  target: string;
}

export interface IConstraintTemplateSpecStatusPod {
  id: string;
  observedGeneration: number;
  operations: string[];
  templateUID: string;
}

export interface IConstraintTemplate {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace: string;
    creationTimestamp: string;
    labels: {
      [key: string]: string;
    };
    annotations?: {
      [key: string]: string;
    };
  };
  spec: {
    crd: {
      spec: {
        names: {
          kind: string;
        };
        validation: any;
      };
    };
    targets: IConstraintTemplateSpecTarget[];
  };
  status: {
    byPod: IConstraintTemplateSpecStatusPod[];
    created: boolean;
  };
}

export interface IRelatedConstraints {
  [key: string]: IConstraint[];
}

export interface IConstraintTemplateResponse {
  constrainttemplates: IConstraintTemplate[];
  constraints_by_constrainttemplates: IRelatedConstraints;
}
