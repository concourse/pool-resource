# pool-resource

*a pool of locks (modeling semaphores)*

Allows you to lock environments, pipeline flow, or other entities which have to
be interacted with in a serial manner.

This resource is backed by a Git repository, so the configuration is largely the
same as the git-resource.

## Git Repository Structure

```
.
├── aws
│   ├── claimed
│   │   ├── .gitkeep
│   │   └── env-2
│   └── unclaimed
│       ├── .gitkeep
│       └── env-1
├── ping-pong-tables
│   ├── claimed
│   │   └── .gitkeep
│   └── unclaimed
│       ├── .gitkeep
│       └── north-table
└── vsphere
    ├── claimed
    │   ├── .gitkeep
    │   └── f3cb3823-a45a-49e8-ab41-e43268494205
    └── unclaimed
        ├── .gitkeep
        └── 83ed9977-3a10-4c49-a818-2d7a37693da7
```

This structure represents 3 pools of locks, `aws`, `ping-pong-tables`, and
`vsphere`. The `.gitkeep` files are required to keep the `unclaimed` and
`claimed` directories track-able by Git if there are no files in them.

You will need to mirror this structure in your own lock repository. In other words, initialize an empty repository
and create one directory in the root of the repository for each pool of locks (e.g. `aws`).  Inside of each lock pool directory, create one directory
named `claimed` and one directory named `unclaimed`. Inside each `claimed` and `unclaimed` directory, create an empty
filed named `.gitkeep`. Finally, create individual locks by making an empty file inside of the `unclaimed` directory
(assuming you want your lock to be unclaimed by default) with the desired name of the lock (e.g. `env-1`).

## Source Configuration

* `uri`: *Required.* The location of the repository.

* `branch`: *Required.* The branch to track.

* `pool`: *Required.* The logical name of your pool of things to lock.

* `private_key`: *Optional.* Private key to use when pulling/pushing. Ensure it does not require a password.
    Example:
    ```
    private_key: |
      -----BEGIN RSA PRIVATE KEY-----
      MIIEowIBAAKCAQEAtCS10/f7W7lkQaSgD/mVeaSOvSF9ql4hf/zfMwfVGgHWjj+W
      <Lots more text>
      DWiJL+OFeg9kawcUL6hQ8JeXPhlImG6RTUffma9+iGQyyBMCGd1l
      -----END RSA PRIVATE KEY-----
    ```

* `username`: *Optional.* Username for HTTP(S) auth when pulling/pushing.
  This is needed when only HTTP/HTTPS protocol for git is available (which does not support private key auth) and auth is required.

* `password`: *Optional.* Password for HTTP(S) auth when pulling/pushing.

* `retry_delay`: *Optional.* If specified, dictates how long to wait until
  retrying to acquire a lock or release a lock. The default is 10 seconds.
  Valid values: `60s`, `90m`, `1h`.

### Example

Fetching a repo with only 100 commits of history:

``` yaml
- get: source-code
  params: {depth: 100}
```

## Behavior

### `check`: Check for changes to the pool.

The repository is cloned (or pulled if already present), and any commits made to the specified pool from the given version on are returned. If no version is
given, the ref for `HEAD` is returned.


### `in`: Fetch an acquired lock.


Outputs 2 files:

* `metadata`: Contains the contents of whatever was in your lock file. This is
  useful for environment configuration settings.

* `name`: Contains the name of lock that was acquired.

#### Parameters

* `depth`: *Optional.* If a positive integer is given, *shallow* clone the
  repository using the `--depth` option.

### `out`: Acquire, release, add, or remove a lock.

Performs one of the following actions to change the state of the pool.

#### Parameters

One of the following is required.

* `acquire`: If true, we will attempt to move a randomly chosen lock from the
  pool's unclaimed directory to the claimed directory. Acquiring will retry
  until a lock becomes available.

* `claim`: If set, the specified lock from the pool will be acquired, rather
  than a random one (as in `acquire`). Like `acquire`, claiming will retry
  until the specific lock becomes available.

* `release`: If set, we will release the lock by moving it from claimed to
  unclaimed. The value is the path of the lock to release (a directory
  containing `name` and `metadata`), which typically is just the step that
  provided the lock (either a `get` to pass one along or a `put` to acquire).

  Note: the lock must be available in your job before you can release it. In
  other words, a `get` step to fetch metadata about the lock is necessary
  before a `put` step can release the lock.

* `add`: If set, we will add a new lock to the pool in the unclaimed state. The
  value is the path to a directory containing the files `name` and `metadata`
  which should contain the name of your new lock and the contents you would like
  in the lock, respectively.

* `add_claimed`: Exactly the same as the `add` param, but adds a lock to the
  pool in the *claimed* state. 

* `remove`: If set, we will remove the given lock from the pool. The value is
  the same as `release`. This can be used for e.g. tearing down an environment,
  or moving a lock between pools by using `add` with a different pool in a
  second step.

* `update`: If set, we will update an existing lock in the pool.

  * If the existing lock is in the unclaimed state we will update it with the
    contents of the `metadata` file.
  * If no such lock is present in either the claimed or unclaimed state we add a
    new lock to the pool in the unclaimed state.
  * If the lock is in the claimed state we will wait for it to be unclaimed and
    proceed to update it as above.

  The value is the path to a directory containing the files `name` and
  `metadata` which should contain the name of your new lock and the contents you
  would like in the lock, respectively.

## Example Concourse Configuration

The following example pipeline models acquiring, passing through, and releasing
a lock based on the example git repository structure:

```
resources:
- name: aws-environments
  type: pool
  source:
    uri: git@github.com:concourse/locks.git
    branch: master
    pool: aws
    private_key: |
      -----BEGIN RSA PRIVATE KEY-----
      MIIEowIBAAKCAQEAtCS10/f7W7lkQaSgD/mVeaSOvSF9ql4hf/zfMwfVGgHWjj+W
      <Lots more text>
      DWiJL+OFeg9kawcUL6hQ8JeXPhlImG6RTUffma9+iGQyyBMCGd1l
      -----END RSA PRIVATE KEY-----

jobs:
- name: deploy-aws
  plan:
    - put: aws-environments
      params: {acquire: true}
    - task: deploy-aws
      file: my-scripts/deploy-aws.yml

- name: test-aws
  plan:
    - get: aws-environments
      passed: [deploy-aws]
    - task: test-aws
      file: my-scripts/test-aws.yml
    - put: aws-environments
      params: {release: aws-environments}
```


### Managing multiple locks

The parameter for `release` is the name of the step whose lock to release.
Normally this is just the same as the name of the resource, but if you have
custom names for `put` (for example if you need to acquire multiple instances
of the same pool), you would put that name instead. For example:

```
- name: test-multi-aws
  plan:
    - put: environment-1
      resource: aws-environments
      params: {acquire: true}
    - put: environment-2
      resource: aws-environments
      params: {acquire: true}
    - task: test-multi-aws
      file: my-scripts/test-multi-aws.yml
    - put: aws-environments
      params: {release: environment-1}
    - put: aws-environments
      params: {release: environment-2}
```

## Development

### Prerequisites

* golang is *required* - version 1.9.x is tested; earlier versions may also
  work.
* docker is *required* - version 17.06.x is tested; earlier versions may also
  work.
* godep is used for dependency management of the golang packages.

### Running the tests

The tests have been embedded with the `Dockerfile`; ensuring that the testing
environment is consistent across any `docker` enabled platform. When the docker
image builds, the test are run inside the docker container, on failure they
will stop the build.

Run the tests with the following commands for both `alpine` and `ubuntu` images:

```sh
docker build -t pool-resource -f dockerfiles/alpine/Dockerfile .
docker build -t pool-resource -f dockerfiles/ubuntu/Dockerfile .
```

### Contributing

Please make all pull requests to the `master` branch and ensure tests pass
locally.
