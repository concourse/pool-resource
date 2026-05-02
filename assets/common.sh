export TMPDIR=${TMPDIR:-/tmp}

load_pubkey() {
  local private_key_path=$TMPDIR/git-resource-private-key
  local private_key_user=$(jq -r '.source.private_key_user // empty' <<< "$1")
  local forward_agent=$(jq -r '.source.forward_agent // false' <<< "$1")
  local passphrase="$(jq -r '.source.private_key_passphrase // empty' <<< "$1")"
  local uri=$(jq -r '.source.uri // ""' <<< "$1")

  (jq -r '.source.private_key // empty' <<< "$1") > $private_key_path

  if [ -s $private_key_path ]; then
    chmod 0600 $private_key_path

    # create or re-initialize ssh-agent
    init_ssh_agent

    SSH_ASKPASS_REQUIRE=force SSH_ASKPASS=$(dirname $0)/askpass.sh GIT_SSH_PRIVATE_KEY_PASS="$passphrase" DISPLAY= ssh-add $private_key_path > /dev/null

    mkdir -p ~/.ssh
    cat > ~/.ssh/config <<EOF
StrictHostKeyChecking no
LogLevel quiet
EOF

    # Handle ssh:// URLs with custom ports
    if [[ "$uri" =~ ^ssh://([^@]+@)?([^:/]+):([0-9]+) ]]; then
      local ssh_host="${BASH_REMATCH[2]}"
      local ssh_port="${BASH_REMATCH[3]}"

      cat >> ~/.ssh/config <<EOF

Host $ssh_host
  Port $ssh_port
EOF
    fi

    if [ ! -z "$private_key_user" ]; then
      cat >> ~/.ssh/config <<EOF
User $private_key_user
EOF
    fi

    if [ "$forward_agent" = "true" ]; then
      cat >> ~/.ssh/config <<EOF
ForwardAgent yes
EOF
    fi

    chmod 0600 ~/.ssh/config
  fi
}

init_ssh_agent() {
  # validate if ssh-agent exist
  set +e
  ssh-add -l &> /dev/null
  exit_code=$?
  set -e

  if [[ ${exit_code} -eq 2 ]]; then
    # ssh-agent does not exist, create ssh-agent
    eval $(ssh-agent) > /dev/null 2>&1
    trap "kill $SSH_AGENT_PID" EXIT
  else
    # ssh-agent exist, remove all identities
    ssh-add -D &> /dev/null
  fi

}


configure_https_tunnel() {
  tunnel=$(jq -r '.source.https_tunnel // empty' <<< "$1")

  if [ ! -z "$tunnel" ]; then
    host=$(echo "$tunnel" | jq -r '.proxy_host // empty')
    port=$(echo "$tunnel" | jq -r '.proxy_port // empty')
    user=$(echo "$tunnel" | jq -r '.proxy_user // empty')
    password=$(echo "$tunnel" | jq -r '.proxy_password // empty')

    pass_file=""
    if [ ! -z "$user" ]; then
      cat > ~/.ssh/tunnel_config <<EOF
proxy_user = $user
proxy_passwd = $password
EOF
      chmod 0600 ~/.ssh/tunnel_config
      pass_file="-F ~/.ssh/tunnel_config"
    fi

    if [[ ! -z $host && ! -z $port ]]; then
      echo "ProxyCommand /usr/bin/proxytunnel $pass_file -p $host:$port -d %h:%p" >> ~/.ssh/config
    fi
  fi
}


configure_credentials() {
  local username=$(jq -r '.source.username // ""' <<< "$1")
  local password=$(jq -r '.source.password // ""' <<< "$1")

  rm -f $HOME/.netrc

  if [ "$username" != "" -a "$password" != "" ]; then
    echo "default login $username password $password" > $HOME/.netrc
  fi
}

configure_git_global() {
  local git_config_payload="$1"
  eval $(echo "$git_config_payload" | \
    jq -r ".[] | \"git config --global '\\(.name)' '\\(.value)'; \"")
}

configure_git_ssl_verification() {
  skip_ssl_verification=$(jq -r '.source.skip_ssl_verification // false' <<< "$1")
  if [ "$skip_ssl_verification" = "true" ]; then
    export GIT_SSL_NO_VERIFY=true
  fi
}
