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


## Source Configuration

* `uri`: *Required.* The location of the repository.

* `branch`: *Required.* The branch to track.

* `pool`: *Required.* The logical name of your pool of things to lock.

* `private_key`: *Optional.* Private key to use when pulling/pushing.
    Example:
    ```
    private_key: |
      -----BEGIN RSA PRIVATE KEY-----
      MIIEowIBAAKCAQEAtCS10/f7W7lkQaSgD/mVeaSOvSF9ql4hf/zfMwfVGgHWjj+W
      <Lots more text>
      DWiJL+OFeg9kawcUL6hQ8JeXPhlImG6RTUffma9+iGQyyBMCGd1l
      -----END RSA PRIVATE KEY-----
    ```

* `retry_delay`: *Optional.* If specified, dictates how long to wait until
  retrying to acquire a lock or release a lock. The default is 10 seconds.


## Behavior

### `check`: Check for changes to the pool.

The repository is cloned (or pulled if already present), and any commits made
after the given version for the specified pool are returned. If no version is
given, the ref for `HEAD` is returned.


### `in`: Fetch an acquired lock.

Outputs 2 files:

* `metadata`: Contains the contents of whatever was in your lock file. This is
  useful for environment configuration settings.

* `name`: Contains the name of lock that was acquired.


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

* `add`: If set, we will add a new lock to the pool in the unclaimed state. The
  value is the path to a directory containing the files `name` and `metadata`
  which should contain the name of your new lock and the contents you would like
  in the lock, respectively.

* `remove`: If set, we will remove the given lock from the pool. The value is
  the same as `release`. This can be used for e.g. tearing down an environment,
  or moving a lock between pools by using `add` with a different pool in a
  second step.


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
