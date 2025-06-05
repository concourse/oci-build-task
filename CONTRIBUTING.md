# contributing

This project runs `buildkit` which is most easily run natively on Linux.

The repository contains submodules; they must be initialized first like so:

```sh
git submodule update --init --recursive
```

## building

The `Dockerfile` makes use of the experimental `RUN --mount` flag, enabled by
the following:

```sh
export DOCKER_BUILDKIT=1
```

Building can be done with `docker build` as normal, though if you're planning
to test this as a Concourse task you'll need to tag and push your own image:

```sh
docker build -t myuser/oci-build-task .
docker push myuser/oci-build-task
```

...and then reference `myuser/oci-build-task` in your task.


## running tests

The tests only run on Linux. If your on a non-linux machine, you can use Docker
to quickly build yourself a dev environment by running the following commands:

```sh
$ docker run -it -v ".:/src" --privileged cgr.dev/chainguard/wolfi-base
> cd /src
> apk add bash curl go
> ./scripts/setup-buildkit.sh
```

The tests can be run rootless, though doing so requires `newuidmap` and
`newgidmap` to be installed:

```sh
apt install uidmap
```

Once this is all done, the tests can be run like so:

```sh
./scripts/test
```

> side note: it would be *super cool* to leverage rootless mode to be able to
> run the tests as part of the `Dockerfile` - unfortunately image building
> involves bind-mounting, which `docker build` does not permit.

## pushing to `concourse/oci-build-task`

The pipeline for managing this task is in the [concourse/ci
repo](https://github.com/concourse/ci/blob/master/pipelines/oci-build-task.yml).
The pipeline itself is running in our CI here:
[https://ci.concourse-ci.org/teams/main/pipelines/oci-build-task](https://ci.concourse-ci.org/teams/main/pipelines/oci-build-task)

You can use the `publish-*` jobs to release a new version of the resource.
