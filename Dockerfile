# syntax=docker/dockerfile:1

ARG GO_VERSION=1.21
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
ARG TARGETOS TARGETARCH
WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download -x

RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,target=. \
    GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0 go build -o /bin/server ./main.go

    
FROM gcr.io/distroless/static:nonroot AS final
EXPOSE 8000
ENV ROOT_CA_CERT /bin/DigiCertGlobalRootCA.crt.pem
COPY ./DigiCertGlobalRootCA.crt.pem /bin/
COPY --from=build /bin/server /bin/
ENTRYPOINT [ "/bin/server" ]