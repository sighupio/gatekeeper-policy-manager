# Copyright (c) 2020 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
FROM node:lts-alpine AS node
COPY app/static /static
WORKDIR /static
RUN npm install 


FROM python:3.9-slim
LABEL org.opencontainers.vendor="SIGHUP.io"
LABEL org.opencontainers.image.authors="SIGHUP https://sighup.io"
LABEL org.opencontainers.image.source="https://github.com/sighupio/gatekeeper-policy-manager"

RUN groupadd -r gpm && useradd --no-log-init -r -g gpm gpm 
WORKDIR /app
COPY --chown=gpm ./app /app
COPY --from=node --chown=gpm /static/node_modules /app/static/node_modules
RUN pip install --no-cache-dir -r /app/requirements.txt
USER 999
EXPOSE 8080
CMD ["gunicorn", "--bind=:8080", "--workers=2", "--threads=4", "--worker-class=gthread", "app:app"]
