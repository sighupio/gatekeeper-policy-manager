# Copyright (c) 2023 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
FROM node:lts-alpine AS node
COPY ./web-client /web-client
WORKDIR /web-client
RUN yarn install && yarn cache clean && yarn build

FROM golang:1.20 as go
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
COPY *.go ./
RUN go mod download
RUN go vet -v
RUN CGO_ENABLED=0 go build -o gpm

FROM gcr.io/distroless/static-debian11:nonroot
LABEL org.opencontainers.vendor="SIGHUP.io"
LABEL org.opencontainers.image.authors="SIGHUP https://sighup.io"
LABEL org.opencontainers.image.source="https://github.com/sighupio/gatekeeper-policy-manager"

# RUN groupadd -r gpm && useradd --no-log-init -r -g gpm gpm 
WORKDIR /app
COPY templates ./templates
COPY --from=go ./app/gpm ./gpm
COPY --from=node /web-client/build/ ./static-content/
EXPOSE 8080
CMD ["/app/gpm"]
