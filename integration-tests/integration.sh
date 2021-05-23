#!/bin/bash

set -e

export TMPDIR_ROOT=$(mktemp -d /tmp/git-tests.XXXXXX)

basedir="$(dirname "$0")"
. "$basedir/../test/helpers.sh"
. "$basedir/helpers.sh"

it_can_check_with_empty_tunnel_information() {
  local pool=${1:-"no-auth"}

  output=$(check_uri_with_private_key_and_incomplete_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination "$pool" "${@:2}" 2>&1)
  echo "$output"

  set +e
  ( echo "$output" | grep 'Via localhost:3128 ->' >/dev/null 2>&1 )
  rc=$?
  set -e

  test "$rc" -ne "0"
}

it_can_check_through_a_tunnel() {
  local pool=${1:-"no-auth"}

  output=$(check_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination "$pool" "${@:2}" 2>&1)
  echo "$output"

  (echo "$output" | grep 'Via localhost:3128 ->' >/dev/null 2>&1)
  rc=$?

  test "$rc" -eq "0"
}

it_can_get_through_a_tunnel() {
  local pool=${1:-"no-auth"}

  get_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination "$pool" "${@:2}"
}

it_can_put_acquire_through_a_tunnel() {
  local pool=${1:-"no-auth"}

  put_acquire_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination "$pool" "${@:2}"
}

it_can_put_release_through_a_tunnel() {
  local pool=${1:-"no-auth"}

  get_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination "$pool" "${@:2}"
  put_release_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR destination "$pool" "${@:2}"
}

it_can_check_through_a_tunnel_with_auth() {
  it_can_check_through_a_tunnel with-auth "$basedir/tunnel/auth"
}

it_can_get_through_a_tunnel_with_auth() {
  it_can_get_through_a_tunnel with-auth "$basedir/tunnel/auth"
}

it_can_put_acquire_through_a_tunnel_with_auth() {
  it_can_put_acquire_through_a_tunnel with-auth "$basedir/tunnel/auth"
}

it_can_put_release_through_a_tunnel_with_auth() {
  it_can_put_release_through_a_tunnel with-auth "$basedir/tunnel/auth"
}

it_cant_check_through_a_tunnel_without_auth() {
  set +e
  test_auth_failure "$(check_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination no-auth 2>&1)"
}

it_cant_get_through_a_tunnel_without_auth() {
  set +e
  test_auth_failure "$(get_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination no-auth 2>&1)"
}

it_cant_put_acquire_through_a_tunnel_without_auth() {
  # The standard error is not captured by the 'out' command when cloning a repository,
  # thus making it impossible to grep a more specific error message.
  set +e
  test_failure "$(put_acquire_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination no-auth "$@" 2>&1)" 'exit status 128'
}

it_cant_put_release_through_a_tunnel_without_auth() {
  local pool="no-auth"

  # Get with auth
  get_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR/destination "$pool" "$basedir/tunnel/auth"

  # Release without auth
  # The standard error is not captured by the 'out' command when cloning a repository,
  # thus making it impossible to grep a more specific error message.
  set +e
  test_failure "$(put_release_uri_with_private_key_and_tunnel_info "$(dirname "$0")/ssh/test_key" $TMPDIR destination "$pool" 2>&1)" 'exit status 128'
}

init_integration_tests $basedir
run it_can_check_with_empty_tunnel_information
run_with_unauthenticated_proxy "$basedir" it_can_check_through_a_tunnel
run_with_unauthenticated_proxy "$basedir" it_can_get_through_a_tunnel
run_with_unauthenticated_proxy "$basedir" it_can_put_acquire_through_a_tunnel
run_with_unauthenticated_proxy "$basedir" it_can_put_release_through_a_tunnel
run_with_unauthenticated_proxy "$basedir" it_can_check_through_a_tunnel_with_auth
run_with_unauthenticated_proxy "$basedir" it_can_get_through_a_tunnel_with_auth
run_with_unauthenticated_proxy "$basedir" it_can_put_acquire_through_a_tunnel_with_auth
run_with_unauthenticated_proxy "$basedir" it_can_put_release_through_a_tunnel_with_auth

run_with_authenticated_proxy "$basedir" it_can_check_through_a_tunnel_with_auth
run_with_authenticated_proxy "$basedir" it_can_get_through_a_tunnel_with_auth
run_with_authenticated_proxy "$basedir" it_can_put_acquire_through_a_tunnel_with_auth
run_with_authenticated_proxy "$basedir" it_can_put_release_through_a_tunnel_with_auth
run_with_authenticated_proxy "$basedir" it_cant_check_through_a_tunnel_without_auth
run_with_authenticated_proxy "$basedir" it_cant_get_through_a_tunnel_without_auth
run_with_authenticated_proxy "$basedir" it_cant_put_acquire_through_a_tunnel_without_auth
run_with_authenticated_proxy "$basedir" it_cant_put_release_through_a_tunnel_without_auth
