# syntax=docker/dockerfile:1
ARG base_image=cgr.dev/chainguard/wolfi-base
ARG builder_image=concourse/golang-builder

ARG BUILDPLATFORM
FROM --platform=${BUILDPLATFORM} ${builder_image} AS builder

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH

COPY . /src
WORKDIR /src
RUN --mount=type=cache,target=/root/.cache/go-build go mod download
ENV CGO_ENABLED=0
RUN go build -o /assets/task ./cmd/task
RUN go build -o /assets/build ./cmd/build

FROM ${base_image} AS task
ARG BUILDKIT_VERSION=0.22.0
RUN apk --no-cache add "buildkitd=~${BUILDKIT_VERSION}" "buildctl=~${BUILDKIT_VERSION}"
COPY --from=builder /assets/task /usr/bin/
COPY --from=builder /assets/build /usr/bin/
COPY bin/setup-cgroups /usr/bin/
ENTRYPOINT ["task"]

FROM task
