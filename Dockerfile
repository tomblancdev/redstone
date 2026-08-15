# ⚡ the kernel as an image: one static binary FROM scratch, nothing else.
# The build stage runs native and cross-compiles via TARGETARCH, so
# multi-arch buildx needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags '-s -w' -o /redstone ./cmd/redstone

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/tomblancdev/redstone" \
      org.opencontainers.image.description="a capability kernel — bind capabilities, never products; verified before powered" \
      org.opencontainers.image.licenses="MIT"
# https probes need CA roots; the binary needs nothing else.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /redstone /redstone
ENTRYPOINT ["/redstone"]
CMD ["serve"]
