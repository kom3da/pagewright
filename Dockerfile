# syntax=docker/dockerfile:1

# Pagewright image: the Go binary with the admin UI embedded, plus the
# extended Pagefind build for Japanese site search.
#
#   docker build -t pagewright .                  # build everything from source
#   docker build --target release ...             # GoReleaser: copy a prebuilt binary
#
# Keep the tool versions in line with mise.toml.
ARG GO_VERSION=1.27
ARG NODE_VERSION=24
ARG DEBIAN_RELEASE=trixie

# --- Admin UI (architecture independent) -----------------------------------
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-${DEBIAN_RELEASE}-slim AS admin
ARG PNPM_VERSION=12.6.0
RUN npm install -g pnpm@${PNPM_VERSION}
WORKDIR /src/web/admin
COPY web/admin/package.json web/admin/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY web/admin/ ./
RUN pnpm build

# --- Go binary (cross-compiled on the build platform) ----------------------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-${DEBIAN_RELEASE} AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=admin /src/web/admin/dist web/admin/dist
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags embedadmin -trimpath -ldflags "-s -w" -o /out/pagewright ./cmd/pagewright

# --- Pagefind (extended build, static musl binary) -------------------------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-${DEBIAN_RELEASE} AS pagefind
ARG PAGEFIND_VERSION=1.6.0
ARG PAGEFIND_SHA256_AMD64=20a992fd64af2ee4fbf39b99245a1135303b6813f79af7ac72a5e27247919b07
ARG PAGEFIND_SHA256_ARM64=46bc248d9b9fa361044d90d7adce4e7fb42156da5256c16da3c7ca6fbc15c9eb
ARG TARGETARCH
RUN set -eu; \
    case "$TARGETARCH" in \
      amd64) triple=x86_64-unknown-linux-musl; sum=$PAGEFIND_SHA256_AMD64 ;; \
      arm64) triple=aarch64-unknown-linux-musl; sum=$PAGEFIND_SHA256_ARM64 ;; \
      *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    file=pagefind_extended-v${PAGEFIND_VERSION}-${triple}.tar.gz; \
    curl -fsSL -o /tmp/$file https://github.com/Pagefind/pagefind/releases/download/v${PAGEFIND_VERSION}/$file; \
    echo "$sum  /tmp/$file" | sha256sum -c -; \
    mkdir -p /out; \
    tar -xzf /tmp/$file -C /out pagefind_extended; \
    chmod 0755 /out/pagefind_extended

# --- Runtime base ----------------------------------------------------------
FROM debian:${DEBIAN_RELEASE}-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 pagewright \
    && useradd --system --uid 10001 --gid 10001 --home-dir /data --shell /usr/sbin/nologin pagewright \
    && mkdir -p /data \
    && chown pagewright:pagewright /data
COPY --from=pagefind /out/pagefind_extended /usr/local/bin/pagefind_extended
ENV PAGEWRIGHT_DATA_DIR=/data \
    PAGEWRIGHT_ADDR=:8080 \
    PAGEWRIGHT_PAGEFIND=/usr/local/bin/pagefind_extended \
    # nginx serves the public site in production. The app treats an empty
    # value as unset (and would listen on :8081), so keep the preview on
    # loopback where nothing outside the container can reach it.
    PAGEWRIGHT_SITE_ADDR=127.0.0.1:8081
# `import-wxr` defaults to --media-root ./data/media, which is /data/media from here.
WORKDIR /
VOLUME /data
EXPOSE 8080
USER 10001:10001
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["curl", "-fsS", "-o", "/dev/null", "http://127.0.0.1:8080/healthz"]
ENTRYPOINT ["pagewright"]
CMD ["serve"]

# --- Release image (GoReleaser puts the binary at <os>/<arch>/pagewright) --
FROM runtime AS release
ARG TARGETPLATFORM
COPY --chmod=0755 ${TARGETPLATFORM}/pagewright /usr/local/bin/pagewright

# --- Default: built from source --------------------------------------------
FROM runtime
COPY --from=gobuild /out/pagewright /usr/local/bin/pagewright
