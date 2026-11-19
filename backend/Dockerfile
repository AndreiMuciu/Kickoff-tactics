# Built on the runner's own architecture and cross-compiled by Go, so the
# linux/amd64 + linux/arm64 image needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# No shell, no package manager, non-root user; CA certificates included
# (needed for MongoDB Atlas and SMTP over TLS).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
ENV ADDR=0.0.0.0:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
