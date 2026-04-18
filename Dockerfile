FROM oven/bun:1@sha256:0733e50325078969732ebe3b15ce4c4be5082f18c4ac1a0f0ca4839c2e4e42a7 AS builder

WORKDIR /build
COPY web/package.json .
COPY web/bun.lock .
RUN bun install
COPY ./web .
COPY ./VERSION .

# Frontend build is memory-heavy (~18k modules). Tune for small build VMs / BuildKit limits:
#   docker buildx build --build-arg NODE_MEMORY_MB=3072 ...
ARG NODE_MEMORY_MB=2048
ENV DISABLE_CODE_INSPECTOR=true \
    ROLLUP_MAX_PARALLEL=2 \
    NODE_OPTIONS="--max-old-space-size=${NODE_MEMORY_MB}"

RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat VERSION) bun run build

FROM golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

# # Default mirrors work when proxy.golang.org is slow or unreachable (e.g. some CN / DNS setups).
# # Override: docker buildx build --build-arg GOPROXY=https://proxy.golang.org,direct ...
# ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
# ARG GOSUMDB=sum.golang.google.cn
# ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}

# use direct mode to download go modules, bypassing the proxy
ENV GOPROXY=direct
# optional, to skip checksum verification
ENV GOSUMDB=off   


WORKDIR /build

# GOPROXY=direct clones modules with git; golang:alpine does not ship git by default.
RUN apk add --no-cache git ca-certificates

# Wait for the frontend stage before Go work so Vite + go mod/build do not peak memory together.
COPY --from=builder /build/dist/index.html /tmp/spa-index.html

ADD go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=builder /build/dist ./web/dist
RUN go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY --from=builder2 /build/new-api /
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
