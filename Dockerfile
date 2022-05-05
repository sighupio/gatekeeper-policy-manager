# Copyright (c) 2022 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
FROM node:lts-alpine AS node
COPY app/static /static
COPY app/web-client /web-client
WORKDIR /static
RUN yarn install
WORKDIR /web-client
RUN yarn install
RUN yarn build


FROM python:3.10-slim
LABEL org.opencontainers.vendor="SIGHUP.io"
LABEL org.opencontainers.image.authors="SIGHUP https://sighup.io"
LABEL org.opencontainers.image.source="https://github.com/sighupio/gatekeeper-policy-manager"

RUN groupadd -r gpm && useradd --no-log-init -r -g gpm gpm 
WORKDIR /app
COPY --chown=gpm ./app /app
COPY --from=node --chown=gpm /static/node_modules /app/static/node_modules
COPY --from=node --chown=gpm /web-client/build /app/static/webapp
RUN pip install --no-cache-dir -r /app/requirements.txt
USER 999
EXPOSE 8080
CMD ["gunicorn", "--bind=:8080", "--workers=2", "--threads=4", "--worker-class=gthread", "app:app"]
