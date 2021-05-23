#!/usr/bin/env bash

set -e -u
set -o pipefail

basedir="$(dirname "$0")"
dest=test_repo_clone

# This is the directory structure expected by the integration tests:
#.
#├── no-auth
#│   ├── claimed
#│   │   └── .gitkeep
#│   └── unclaimed
#│       ├── .gitkeep
#│       └── lock
#└── with-auth
#    ├── claimed
#    │   └── .gitkeep
#    └── unclaimed
#        ├── .gitkeep
#        └── lock
build_lock_structure() {
  local dir=$1
  rm -rf "${dir}"

  mkdir -p "${dir}/unclaimed"
  touch "${dir}/unclaimed/.gitkeep"
  touch "${dir}/unclaimed/lock"

  mkdir -p "${dir}/claimed"
  touch "${dir}/claimed/.gitkeep"

  git add ${dir}
}

clone_repo() {
  local repo=$1
  local branch=$2
  local dest=$3

  git clone -b "$branch" "$repo" "$dest"
  pushd "$dest" >/dev/null
    git config pull.rebase false
  popd >/dev/null
}

main() {
  pushd "${basedir}" >/dev/null
    repo=$(cut -d '#' -f1 <test_repo)
    branch=$(cut -d '#' -f2 <test_repo)
    [[ "$repo" == "$branch" ]] && branch=main

    # Clone the repo if required
    if [[ ! -d $dest ]]; then
      clone_repo "$repo" "${branch}" "$dest"
    fi

    pushd "$dest" >/dev/null
      existing_repo=$(git config --get remote.origin.url)
      if [[ "$repo" != "$existing_repo" ]]; then
        echo "error: unexpected repo, '$repo' != '$existing_repo'"
        exit 1
      fi

      existing_branch=$(git branch --show-current)
      if [[ "$branch" != "$existing_branch" ]]; then
        echo "error: unexpected branch, '$branch' != '$existing_branch'"
        exit 1
      fi

      git pull
      build_lock_structure "no-auth"
      build_lock_structure "with-auth"

      if [[ $(git status --porcelain) ]]; then
        git commit -m "reset"
        git push
      else
        echo "no changes"
      fi
    popd >/dev/null
  popd >/dev/null
}

main "$@"
