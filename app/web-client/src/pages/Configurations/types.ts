/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export interface IGVK {
  group: string;
  version: string;
  kind: string;
}

export interface ITrace {
  user: string;
  kind: IGVK;
  dump: string;
}

export interface IConfigSpecSync {
  syncOnly: IGVK[];
}

export interface IConfigSpecValidation {
  traces: ITrace[];
}

export interface IConfigSpecMatch {
  processes: string[];
  excludedNamespaces: string[];
}

export interface IConfigSpecReadiness {
  statsEnabled: boolean;
}

export interface IConfigSpec {
  sync?: IConfigSpecSync;
  validation?: IConfigSpecValidation;
  match?: IConfigSpecMatch[];
  readiness?: IConfigSpecReadiness;
}

export interface IConfig {
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
