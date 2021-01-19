# syntax = docker/dockerfile:1.0-experimental
FROM busybox
RUN --mount=type=secret,id=secret test "$(cat /run/secrets/secret)" = "hello-world"
