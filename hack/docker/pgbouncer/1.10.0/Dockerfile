FROM alpine:3.10
RUN set -x \
  && apk add --update --no-cache ca-certificates pgbouncer postgresql-client
USER 65535:65535
cmd ["pgbouncer", "/etc/config/pgbouncer.ini"]