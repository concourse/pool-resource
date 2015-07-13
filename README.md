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

### `check`: Check for new commits.

The repository is cloned (or pulled if already present), and any commits made
after the given version for the specified pool are returned. If no version is
given, the ref for `HEAD` is returned.


### `in`: Clone the repository, at the given ref.

Outputs 2 files:

* `metadata`: Contains the contents of whatever was in your lock file. This is
  useful for environment configuration settings.

* `name`: Contains the name of lock that was acquired.


### `out`: Push to a repository.

Either claims or releases a lock and pushes to the repository.

#### Parameters

One of the following is required.

* `acquire`: If true, we will attempt to move a file from your unclaimed lock
  pool to claimed. We will try to acquire a lock until one is made available.

* `release`: If set, we will release the lock by moving it from claimed to
  unclaimed. The value is the name of the get task that had the lock passed
  from the previous job. This means that you need to *get* the pool-resource
  first **before** releasing a lock.

* `add`: If set, we will add a new lock to the pool in the unclaimed state. The
  value is the path to a directory containing the files `name` and `metadata`
  which should contain the name of your new lock and the contents you would like
  in the lock, respectively.


## Example Concourse Configuration

The following example pipeline models acquiring, passing through, and releasing
a lock based on the example git repository structure (this sample does not include a `private_key` key-value pair in the `source` configuration; however, when you create your pipeline, you will want to include one to avoid an `error acquiring lock: exit status 128` pipeline failure):

```
resources:
- name: aws-environments
  type: pool
  source:
    uri: git@github.com:concourse/locks.git
    branch: master
    pool: aws

jobs:
- name: deploy-aws
  plan:
    - put: aws-environments
      params:
        acquire: true
    - task: deploy-aws
      file: my-scripts/deploy-aws.yml

- name: test-aws
  plan:
    - get: aws-environments
      passed: [deploy-aws]
    - task: test-aws
      file: my-scripts/test-aws.yml
    - put: aws-environments
      params:
        release: aws-environments
```
