# syntax = docker/dockerfile:experimental

FROM concourse/golang-builder AS builder
  WORKDIR /src
  COPY go.mod /src/go.mod
  COPY go.sum /src/go.sum
  RUN --mount=type=cache,target=/root/.cache/go-build go get -d ./...
  COPY . /src
  ENV CGO_ENABLED 0
  RUN go build -o /assets/task ./cmd/task
  RUN go build -o /assets/build ./cmd/build

FROM moby/buildkit:master AS task
  COPY --from=builder /assets/task /usr/bin/
  COPY --from=builder /assets/build /usr/bin/
  COPY bin/setup-cgroups /usr/bin/
  ENTRYPOINT ["task"]

FROM task
