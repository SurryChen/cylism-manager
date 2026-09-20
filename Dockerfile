FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
RUN go mod download
COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 go build -o /cylism-manager ./cmd/platform/
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /cylism-cli ./cmd/cylism-cli/

FROM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM alpine:3.20
RUN apk --no-cache add ca-certificates curl openssh

WORKDIR /app
COPY --from=builder /cylism-manager .
COPY --from=builder /cylism-cli /usr/local/lib/cylism/runtime-tools/cylism-cli
COPY --from=web-builder /web/dist ./web/dist
COPY config/ ./config/
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG CYLISM_CLI_VERSION=dev
RUN case "${CYLISM_CLI_VERSION}" in ""|*[!A-Za-z0-9._-]*) exit 1 ;; esac \
    && cli_dir=/usr/local/lib/cylism/runtime-tools \
    && checksum=$(sha256sum "${cli_dir}/cylism-cli" | awk '{print $1}') \
    && size=$(wc -c < "${cli_dir}/cylism-cli" | tr -d '[:space:]') \
    && printf '{"version":"%s","platform":"%s-%s","size":%s,"sha256":"%s"}\n' "${CYLISM_CLI_VERSION}" "${TARGETOS}" "${TARGETARCH}" "${size}" "${checksum}" > "${cli_dir}/cylism-cli.manifest.json" \
    && chmod 0555 "${cli_dir}/cylism-cli" \
    && chmod 0444 "${cli_dir}/cylism-cli.manifest.json" \
    && mkdir -p /data

EXPOSE 8080
CMD ["./cylism-manager"]
