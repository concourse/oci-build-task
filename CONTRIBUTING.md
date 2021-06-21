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

The tests only run on Linux.

The tests can be run rootless, though doing so requires `newuidmap` and
`newgidmap` to be installed:

```sh
apt install uidmap
```

Once this is all done, the tests can be run like so:

```sh
./scripts/test # repeat as needed
```

> side note: it would be *super cool* to leverage rootless mode to be able to
> run the tests as part of the `Dockerfile` - unfortunately image building
> involves bind-mounting, which `docker build` does not permit.

## pushing to `concourse/oci-build-task`

This repo is automated using GitHub Actions.

Why GitHub Actions and not Concourse, you ask? Just 'cause! Figured this was a
good use case to kick the tires on it, especially since their actions run in
real VMs, so it eliminates the question of testing containers-in-questions.

Anyhow - whenever commits are pushed to a branch, a new image will be pushed to
`concourse/oci-build-task:${branchname}`.

So to try out the latest changes, point to `concourse/oci-build-task:master`.

Additionally, for each PR an image will be built and pushed to
`concourse/oci-build-task:pr${number}`.

## shipping a new version

Shipping is done by creating a new semver tag, i.e. `v1.2.3`, and pushing it.
The GitHub actions will push to the appropriate semver tags (i.e. `1.2.3`,
`1.2`, `1`), along with `latest`.
