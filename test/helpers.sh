#!/bin/sh

set -e -u

set -o pipefail

resource_dir=/opt/resource

run() {
  export TMPDIR=$(mktemp -d ${TMPDIR_ROOT}/git-tests.XXXXXX)

  echo -e 'running \e[33m'"$@"$'\e[0m...'
  eval "$@" 2>&1 | sed -e 's/^/  /g'
  echo ""
}

init_repo() {
  (
    set -e

    cd $(mktemp -d $TMPDIR/repo.XXXXXX)

    git init -q

    # start with an initial commit
    git \
      -c user.name='test' \
      -c user.email='test@example.com' \
      commit -q --allow-empty -m "init"

    mkdir my_pool
    mkdir my_pool/unclaimed
    mkdir my_pool/claimed

    mkdir my_other_pool
    mkdir my_other_pool/unclaimed
    mkdir my_other_pool/claimed

    touch my_pool/unclaimed/.gitkeep
    touch my_pool/claimed/.gitkeep

    touch my_other_pool/unclaimed/.gitkeep
    touch my_other_pool/claimed/.gitkeep

    git add .
    git commit -q -m "setup lock"

    # print resulting repo
    pwd
  )
}

make_commit_to_file_on_branch() {
  local repo=$1
  local file=$2
  local branch=$3
  local msg=${4-}

  # ensure branch exists
  if ! git -C $repo rev-parse --verify $branch >/dev/null; then
    git -C $repo branch $branch master
  fi

  # switch to branch
  git -C $repo checkout -q $branch

  # modify file and commit
  echo x >> $repo/$file
  git -C $repo add $file
  git -C $repo \
    -c user.name='test' \
    -c user.email='test@example.com' \
    commit -q -m "commit $(wc -l $repo/$file) $msg"

  # output resulting sha
  git -C $repo rev-parse HEAD
}

make_commit_to_file() {
  make_commit_to_file_on_branch $1 $2 master "${3-}"
}

make_commit_to_branch() {
  make_commit_to_file_on_branch $1 some-file $2
}

make_commit() {
  make_commit_to_file $1 some-file
}

make_commit_to_be_skipped() {
  make_commit_to_file $1 some-file "[ci skip]"
}

check_uri() {
  jq -n "{
    source: {
      uri: $(echo $1 | jq -R .),
      branch: \"master\",
      pool: \"my_pool\"
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_ignoring() {
  local uri=$1

  shift

  jq -n "{
    source: {
      uri: $(echo $uri | jq -R .),
      branch: \"master\",
      pool: \"my_pool\"
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_paths() {
  local uri=$1
  shift

  local pool=$1
  shift

  jq -n "{
    source: {
      uri: $(echo $uri | jq -R .),
      branch: \"master\",
      pool: $(echo $pool | jq -R .)
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_from() {
  jq -n "{
    source: {
      uri: $(echo $1 | jq -R .),
      branch: \"master\",
      pool: \"my_pool\"
    },
    version: {
      ref: $(echo $2 | jq -R .)
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_from_paths() {
  local uri=$1
  local ref=$2

  shift 2

  jq -n "{
    source: {
      uri: $(echo $uri | jq -R .),
      branch: \"master\",
      pool: \"my_pool\"
    },
    version: {
      ref: $(echo $ref | jq -R .)
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_with_credentials() {
  jq -n "{
    source: {
      uri: $(echo $1 | jq -R .),
      branch: \"master\",
      pool: \"my_pool\",
      username: $(echo $2 | jq -R .),
      password: $(echo $3 | jq -R .)
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_with_key() {
  local uri=$1
  local key=$2

  jq -n "{
    source: {
      uri: $(echo $uri | jq -R .),
      branch: \"master\",
      pool: \"my_pool\",
      private_key: $(cat $key | jq -s -R .),
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_with_key_and_tunnel() {
  local uri=$1
  local key=$2
  local host=$3
  local port=$4

  jq -n "{
    source: {
      uri: $(echo $uri | jq -R .),
      branch: \"master\",
      pool: \"my_pool\",
      private_key: $(cat $key | jq -s -R .),
      https_tunnel: {
        proxy_host: $(echo $host | jq -R .),
        proxy_port: $(echo $port | jq -R .),
      },
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}

check_uri_with_key_and_tunnel_with_authentication() {
  local uri=$1
  local key=$2
  local host=$3
  local port=$4
  local username=$5
  local password=$6

  jq -n "{
    source: {
      uri: $(echo $uri | jq -R .),
      branch: \"master\",
      pool: \"my_pool\",
      private_key: $(cat $key | jq -s -R .),
      https_tunnel: {
        proxy_host: $(echo $host | jq -R .),
        proxy_port: $(echo $port | jq -R .),
        proxy_user: $(echo $username | jq -R .),
        proxy_password: $(echo $password | jq -R .),
      },
    }
  }" | ${resource_dir}/check | tee /dev/stderr
}
