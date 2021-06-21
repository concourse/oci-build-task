# syntax = docker/dockerfile:1.0-experimental
FROM alpine

RUN apk add --no-cache openssh-client

# shows private key available in ssh agent
RUN --mount=type=ssh,id=my_ssh_key ssh-add -l | grep "SHA256:DFxHFuit9VQtxkBrZWzJhf5OTL5/RwzCJuZjTAPC1DI"
