# builder

A Concourse task to builds Docker images without pushing and without spinning
up a Docker daemon. Currently uses [`img`](http://github.com/genuinetools/img)
for the building and saving.

This repository describes an image which should be used to run a task similar
to `example.yml`.

A stretch goal of this is to support running without `privileged: true`, though
it currently still requires it.


## task config

Concourse doesn't yet have easily distributable task configs (there's an [open
call for an RFC](https://github.com/concourse/rfcs/issues/7)), so we'll just
have to document what you need to set here.

### `image`

The task's image should refer to `concourse/builder-task`. You can either
configure this via `image_resource` or pull it in as part of your pipeline.

### `params`

The following params are required:

* `$REPOSITORY`: the repository to name the image, e.g.
  `concourse/builder-task`.

The following are optional:

* `$TAG` (default `latest`): the tag to apply to the image.

* `$CONTEXT` (default `.`): the path to the directory to build. This should
  refer to one of the inputs.

* `$DOCKERFILE` (default `$CONTEXT/Dockerfile`): the path to the `Dockerfile`
  to build.

* `$BUILD_ARG_*` (default empty): Params that start with `BUILD_ARG_` will be
  translated to `--build-arg` options. For example `BUILD_ARG_foo=bar`, will become
  `--build-arg foo=bar`

### `inputs`

There are no required inputs - your task should just list each artifact it
needs as an input.

### `outputs`

Your task may configure an output called `image`. The saved image tarball will
be written to `image.tar` within the output. This tarball can be passed along
to `docker load`, or uploaded to a registry using the [Registry Image
resource](https://github.com/concourse/registry-image-resource#out-push-an-image-up-to-the-registry-under-the-given-tags).

### `caches`

Build caching can be enabled by configuring a cache named `cache` on the task.

### `run`

Your task should execute the `build` script.


## example

This repo contains an `example.yml`, which builds the image for the builder
itself:

```sh
fly -t dev execute -c example.yml -o image=. -p
docker load -i image.tar
```
