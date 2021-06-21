# syntax = docker/dockerfile:1.0-experimental
FROM alpine

# shows secret from custom secret location:
RUN --mount=type=secret,id=my_ssh_key cat ./id_rsa_test
