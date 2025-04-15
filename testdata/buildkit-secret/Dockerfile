# syntax = docker/dockerfile:1.11
FROM busybox
RUN --mount=type=secret,id=secret test "$(cat /run/secrets/secret)" = "hello-world"
