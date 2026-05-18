ARG GO_VERSION=1.25.0

FROM golang:${GO_VERSION}-bookworm AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-trimpath \
	-ldflags="-s -w -X github.com/alex-poliushkin/theater.version=${VERSION}" \
	-o /out/theater \
	./cmd/theater

RUN mkdir -p /out/rootfs/tmp /out/rootfs/workspace \
	&& chmod 1777 /out/rootfs/tmp \
	&& chmod 0755 /out/rootfs/workspace

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/rootfs/tmp /tmp
COPY --from=build --chown=65532:65532 /out/rootfs/workspace /workspace
COPY --from=build /out/theater /theater

USER 65532:65532
WORKDIR /workspace
ENTRYPOINT ["/theater"]
CMD ["help"]
