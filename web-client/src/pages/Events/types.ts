/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export interface IEvent {
  kind: string;
  eventTime: string;
  firstTimestamp: string;
  lastTimestamp: string;
  count: number;
  message: string;
  reason: string;
  type: string;
  involvedObject: IInvolvedObject
  metadata: {
    name: string;
    creationTimestamp: string;
    annotations?: any;
    labels?: any;
  };
}


export interface IInvolvedObject {
  kind: string;
  name: string;
  namespace: string;
}
