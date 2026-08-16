# Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.


FROM --platform=$BUILDPLATFORM golang:1.26.6 AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download -x
# hadolint ignore=DL3059
# Full context bind: go vet type-checks the packages, which resolves the go:embed directives, so
# the embedded templates and static assets have to be present, not just the .go files.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,target=. \
    go vet -v
# hadolint ignore=DL3059
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /bin/gpm


FROM gcr.io/distroless/static-debian11:nonroot AS target
# GPM hands the browser paths under this subpath (redirects, asset URLs, the login URL). Set it at
# build time with --build-arg PUBLIC_URL=/gpm, or at run time with GPM_BASE_PATH.
ARG PUBLIC_URL=""
ENV GPM_BASE_PATH=$PUBLIC_URL
LABEL org.opencontainers.vendor="SIGHUP.io"
LABEL org.opencontainers.image.authors="SIGHUP https://sighup.io"
LABEL org.opencontainers.image.source="https://github.com/sighupio/gatekeeper-policy-manager"

WORKDIR /app
COPY --from=backend ./bin/gpm ./gpm
EXPOSE 8080
CMD ["/app/gpm"]
