# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
# Numeric UID (distroless "nonroot" = 65532): Kubernetes' runAsNonRoot can only
# verify a numeric user and refuses to start the pod with a named one.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/api"]
