# Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.


FROM --platform=$BUILDPLATFORM node:lts-alpine AS frontend
ARG TARGETOS
ARG TARGETARCH
COPY ./web-client /web-client
WORKDIR /web-client
ENV npm_config_target_arch=${TARGETARCH} npm_config_target_platform=${TARGETOS}
RUN yarn install && yarn cache clean && yarn build


FROM --platform=$BUILDPLATFORM golang:1.20 AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download -x
# hadolint ignore=DL3059
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go vet -v
# hadolint ignore=DL3059
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bin/gpm


FROM gcr.io/distroless/static-debian11:nonroot AS target
LABEL org.opencontainers.vendor="SIGHUP.io"
LABEL org.opencontainers.image.authors="SIGHUP https://sighup.io"
LABEL org.opencontainers.image.source="https://github.com/sighupio/gatekeeper-policy-manager"

WORKDIR /app
COPY templates ./templates
COPY --from=backend ./bin/gpm ./gpm
COPY --from=frontend /web-client/build/ ./static-content/
EXPOSE 8080
CMD ["/app/gpm"]
