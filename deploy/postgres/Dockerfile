# 開発用：pg_bigm を入れた PostgreSQL
FROM postgres:17
# TODO: pg_bigm の最新リリースタグを確認して合わせる（https://github.com/pgbigm/pg_bigm/releases）
ARG PG_BIGM_VERSION=1.2-20250903
RUN apt-get update \
 && apt-get install -y --no-install-recommends build-essential postgresql-server-dev-17 curl ca-certificates \
 && curl -fsSL -o /tmp/pg_bigm.tar.gz https://github.com/pgbigm/pg_bigm/archive/refs/tags/v${PG_BIGM_VERSION}.tar.gz \
 && tar -xzf /tmp/pg_bigm.tar.gz -C /tmp \
 && cd /tmp/pg_bigm-${PG_BIGM_VERSION} && make USE_PGXS=1 && make USE_PGXS=1 install \
 && apt-get purge -y build-essential postgresql-server-dev-17 curl && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/* /tmp/pg_bigm*
