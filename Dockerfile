# Copyright (c) 2020 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
FROM node:lts-alpine AS node
COPY app/static /app
WORKDIR /app
RUN npm install 


FROM tiangolo/uwsgi-nginx-flask:python3.8-alpine
RUN apk --update --no-cache add gcc=9.3.0-r0 linux-headers=4.19.36-r0 musl-dev=1.1.24-r2 libffi-dev=3.2.1-r6 libressl-dev=3.0.2-r0
ENV STATIC_URL /static
ENV STATIC_PATH /app/static
ENV LISTEN_PORT 8080
EXPOSE 8080
COPY ./app/requirements.txt /var/www/requirements.txt
RUN pip install -r /var/www/requirements.txt
COPY ./app /app
COPY --from=node /app/node_modules /app/static/node_modules
