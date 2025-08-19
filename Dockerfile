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

ARG BUILDKIT_VERSION
WORKDIR /buildkit
RUN apk --no-cache add curl
RUN curl -L "https://github.com/moby/buildkit/releases/download/v${BUILDKIT_VERSION}/buildkit-v${BUILDKIT_VERSION}.linux-${TARGETARCH}.tar.gz" -o buildkit.tar.gz && \
    tar xf buildkit.tar.gz

FROM ${base_image} AS task
RUN apk --no-cache add \
    ca-certificates \
    cmd:umount \
    cmd:mount \
    cmd:mountpoint
COPY --from=builder /assets/task /usr/bin/
COPY --from=builder /assets/build /usr/bin/
COPY --from=builder /buildkit/bin/ /usr/bin/
COPY bin/setup-cgroups /usr/bin/
RUN for cmd in task build buildkitd buildctl mount umount mountpoint setup-cgroups; do \
    which $cmd >/dev/null || { echo "$cmd binary not found!"; exit 1; }; \
    done
ENTRYPOINT ["task"]

FROM task
