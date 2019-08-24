# `oci-build` task

![badge](https://ci.concourse-ci.org/api/v1/teams/main/pipelines/builder-task/jobs/test/badge)

A Concourse task for building [OCI
images](https://github.com/opencontainers/image-spec). Currently uses
[`buildkit`](http://github.com/moby/buildkit) for building.

A stretch goal of this is to support running without `privileged: true`, though
it currently still requires it.

## differences from `builder-task`

* simpler and more efficient caching implementation
* does not support configuring `$REPOSITORY` or `$TAG`
  * for running the image with `docker`, a `digest` file is provided which can
    be tagged with `docker tag`
  * for pushing the image, the repository and tag are configured in the
    [`registry-image`
    resource](https://github.com/concourse/registry-image-resource)
* written in Go, with tests!
* uses `buildkit` directly

## task config

The task implementation is available as
[`concourse/oci-build-task`](http://hub.docker.com/r/concourse/oci-build-task),
which is built from [`Dockerfile`](Dockerfile).

This task implementation started as a spike to explore patterns around
[reusable tasks](https://github.com/concourse/rfcs/issues/7) in service of
coming up with ideas for a proper RFC. Until that RFC is written and
implemented, configuration is still done by way of providing your own task
config as follows:

### `image_resource`

First, your task needs to point to the `oci-build-task` image:

```yaml
image_resource:
  type: registry-image
  source:
    repository: concourse/oci-build-task
```

### `params`

Next, any of the following optional parameters may be specified:

* `$CONTEXT` (default `.`): the path to the directory to provide as the context
  for the build.

* `$DOCKERFILE` (default `$CONTEXT/Dockerfile`): the path to the `Dockerfile`
  to build.

* `$BUILD_ARG_*`: params prefixed with `BUILD_ARG_` will be provided as build
  args. For example `BUILD_ARG_foo=bar`, will set the `foo` build arg as `bar`.

* `$BUILD_ARGS_FILE` (default empty): path to a file containing build args in
  the form `foo=bar`, one per line. Empty lines are skipped.

  Example file contents:

  ```
  EMAIL=me@yopmail.com
  HOW_MANY_THINGS=1
  DO_THING=false
  ```

* `$TARGET` (default empty): a target build stage to build.

* `$TARGET_FILE` (default empty): path to a file containing the name of the
  target build stage to build.

* `$UNPACK_ROOTFS` (default `false`): unpack the image as Concourse's image
  format (`rootfs/`, `metadata.json`) for use with the [`image` task step
  option](https://concourse-ci.org/task-step.html#task-step-image).

> Note: this is the main pain point with reusable tasks - env vars are kind of
> an awkward way to configure a task. Once the RFC lands these will turn into a
> JSON structure similar to configuring `params` on a resource, and task params
> will become `env` instead.

### `inputs`

There are no required inputs - your task should just list each artifact it
needs as an input. Typically this is in close correlation with `$CONTEXT`:

```yaml
params:
  CONTEXT: my-image

inputs:
- name: my-image
```

Should your build be dependent on multiple inputs, you may want to leave
`$CONTEXT` as its default (`.`) and set an explicit path to the `$DOCKERFILE`:

```yaml
params:
  DOCKERFILE: my-repo/Dockerfile

inputs:
- name: my-repo
- name: some-dependency
```

It might also make sense to place one input under another, like so:

```yaml
params:
  CONTEXT: my-repo

inputs:
- name: my-repo
- name: some-dependency
  path: my-repo/some-dependency
```

Or, to fully rely on the default behavior and use `path` to wire up the context
accordingly, you could set your primary context as `path: .` and set up any
additional inputs underneath:

```yaml
inputs:
- name: my-repo
  path: .
- name: some-dependency
```

### `outputs`

A single output named `image` may be configured:

```yaml
outputs:
- name: image
```

The output will contain the following files:

* `image.tar`: the OCI image tarball. This tarball can be uploaded to a
  registry using the [Registry Image
  resource](https://github.com/concourse/registry-image-resource#out-push-an-image-up-to-the-registry-under-the-given-tags).

* `digest`: the digest of the OCI config. This file can be used to tag the
  image after it has been loaded with `docker load`, like so:

  ```sh
  docker load -i image/image.tar
  docker tag $(cat image/digest) my-name
  ```

If `$UNPACK_ROOTFS` is configured, the following additional entries will be
created:

* `rootfs/*`: the unpacked contents of the image's filesystem.

* `metadata.json`: a JSON file containing the image's env and user
  configuration.

This is a Concourse-specific format to support using the newly built image for
a subsequent task by pointing the task step's [`image`
option](https://concourse-ci.org/task-step.html#task-step-image) to the output,
like so:

```yaml
plan:
- task: build-image
  output_mapping: {image: my-built-image}
- task: use-image
  image: my-built-image
```

(The `output_mapping` here is just for clarity; alternatively you could just
set `image: image`.)

> Note: at some point Concourse will likely standardize on OCI instead.

### `caches`

Caching can be enabled by caching the `cache` path on the task:

```yaml
caches:
- path: cache
```

### `run`

Your task should run the `build` executable:

```yaml
run:
  path: build
```


## example

This repo contains an `example.yml`, which builds the image for the task
itself:

```sh
fly -t dev execute -c example.yml -o image=. -p
docker load -i image.tar
```
