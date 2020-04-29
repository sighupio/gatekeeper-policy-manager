FROM tiangolo/uwsgi-nginx-flask:python3.6-alpine3.7
RUN apk --update --no-cache add gcc linux-headers musl-dev libffi-dev libressl-dev
ENV STATIC_URL /static
ENV STATIC_PATH /app/static
ENV LISTEN_PORT 8080
EXPOSE 8080
COPY ./app/requirements.txt /var/www/requirements.txt
RUN pip install -r /var/www/requirements.txt
COPY ./app /app