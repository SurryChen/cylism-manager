FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /cylism-manager ./cmd/platform/

FROM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM alpine:3.20
RUN apk --no-cache add ca-certificates curl
WORKDIR /app
COPY --from=builder /cylism-manager .
COPY --from=web-builder /web/dist ./web/dist
COPY config/ ./config/
RUN mkdir -p /data

EXPOSE 8080
CMD ["./cylism-manager"]
