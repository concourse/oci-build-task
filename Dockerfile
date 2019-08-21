# syntax = docker/dockerfile:experimental

FROM concourse/golang-builder AS builder
  WORKDIR /src
  ENV CGO_ENABLED 0
  COPY go.mod /src/go.mod
  COPY go.sum /src/go.sum
  RUN --mount=type=cache,target=/root/.cache/go-build go get -d ./...
  COPY . /src
  RUN go build -o /assets/builder-task ./cmd/builder-task
  RUN go build -o /assets/build ./cmd/build
  RUN set -e; for pkg in $(go list ./...); do \
        go test -o "/tests/$(basename $pkg).test" -c $pkg; \
      done

FROM moby/buildkit AS task
  COPY --from=builder /assets/builder-task /usr/bin/
  COPY --from=builder /assets/build /usr/bin/
  COPY bin/setup-cgroups /usr/bin/
  ENTRYPOINT ["builder-task"]

FROM task
