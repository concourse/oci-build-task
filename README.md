# builder

A Concourse task for building [OCI
images](https://github.com/opencontainers/image-spec). Currently uses
[`buildkit`](http://github.com/moby/buildkit) for building.

This repository describes an image which should be used to run a task similar
to `example.yml`.

A stretch goal of this is to support running without `privileged: true`, though
it currently still requires it.


## task config

Concourse doesn't yet have easily distributable task configs (there's an [open
call for an RFC](https://github.com/concourse/rfcs/issues/7)), so we'll just
have to document what you need to set in your own task config here.

### `image`

The task's image should refer to the
[`concourse/builder-task`](http://hub.docker.com/r/concourse/builder-task)
repository, which is built from [`Dockerfile`](Dockerfile).

You can either configure this via `image_resource` or pull it in as part of
your pipeline.

### `params`

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
  format (`rootfs/`, `metadata.json`) for use with [`image` task step
  option](https://concourse-ci.org/task-step.html#task-step-image).

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

* `rootfs/*` (with `$UNPACK_ROOTFS`): the unpacked contents of the image's
  filesystem.

* `metadata.json` (with `$UNPACK_ROOTFS`): a JSON file containing the image's
  env and user configuration.

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

This repo contains an `example.yml`, which builds the image for the builder
itself:

```sh
fly -t dev execute -c example.yml -o image=. -p
docker load -i image.tar
```
