/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */


export interface IMutationSpecMatch {
  kinds: string[];
  scope: string;
}


export interface IMutationSpec {
  parameters?: any;
  location?: string;
  applyTo?: any;
  match?: IMutationSpecMatch[];
}

export interface IMutation {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    creationTimestamp: string;
    annotations?: any;
  };
  spec?: IMutationSpec;
  status?: any;
}
