# Lock

Locks down a given set of jobs so they run sequentially. This is implemented
with a git repo on the backend, so the configuration is largely the same.


## Source Configuration

* `uri`: *Required.* The location of the repository.

* `branch`: *Required.* The branch to track. If not specified, the repository's
  default branch is assumed.

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

* `paths`: *Optional.* If specified (as a list of glob patterns), only changes
  to the specified files will yield new versions.

* `ignore_paths`: *Optional.* The inverse of `paths`; changes to the specified
  files are ignored.


## Behavior

### `check`: Check for new commits.

The repository is cloned (or pulled if already present), and any commits
made after the given version are returned. If no version is given, the ref
for `HEAD` is returned.


### `in`: Clone the repository, at the given ref.



#### Parameters



### `out`: Push to a repository.


#### Parameters

