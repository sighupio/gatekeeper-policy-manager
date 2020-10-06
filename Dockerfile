# Copyright (c) 2020 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

FROM tiangolo/uwsgi-nginx-flask:python3.8-alpine
RUN apk --update --no-cache add gcc>=6.4.0-r9 linux-headers>=4.4.6-r2 musl-dev>=1.1.19-r11 libffi-dev>=3.2.1-r4 libressl-dev>=2.7.5-r0
ENV STATIC_URL /static
ENV STATIC_PATH /app/static
ENV LISTEN_PORT 8080
EXPOSE 8080
COPY ./app/requirements.txt /var/www/requirements.txt
RUN pip install -r /var/www/requirements.txt
COPY ./app /app
