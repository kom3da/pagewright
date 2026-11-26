# 開発用：pg_bigm を入れた PostgreSQL
FROM postgres:18
ARG PG_BIGM_VERSION=1.2-20261118
RUN apt-get update \
 && apt-get install -y --no-install-recommends build-essential postgresql-server-dev-18 libicu-dev curl ca-certificates \
 && curl -fsSL -o /tmp/pg_bigm.tar.gz https://github.com/pgbigm/pg_bigm/archive/refs/tags/v${PG_BIGM_VERSION}.tar.gz \
 && tar -xzf /tmp/pg_bigm.tar.gz -C /tmp \
 && cd /tmp/pg_bigm-${PG_BIGM_VERSION} && make USE_PGXS=1 && make USE_PGXS=1 install \
 && apt-get purge -y build-essential postgresql-server-dev-18 libicu-dev curl && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/* /tmp/pg_bigm*
