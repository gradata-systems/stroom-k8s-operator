#!/bin/bash

auth_token='/stroom/auth/token'
token_expiry_threshold_seconds=10
base_url="http://localhost:${STROOM_APP_PORT}/api"

function get_auth_token() {
  if [ "$(is_token_expired)" -eq "1" ]; then
    ./start.sh fetch_proc_user_token --outFile="$auth_token"

    if [ -s "$auth_token" ]; then
      # Token was successfully created
      chmod 600 "$auth_token"
      return 0
    else
      exit 1
    fi
  fi
}

function is_token_expired() {
  if [ -s "$auth_token" ]; then
    token=$(cat "$auth_token")

    # Split the token into its three parts: header, payload, and signature
    IFS='.' read -r -a token_parts <<< "$token"

    # Decode the base64-encoded payload
    payload_base64="${token_parts[1]}"
    expiry=$(echo "$payload_base64" | base64 -d | jq -r '.exp')
    expiry=$((expiry - token_expiry_threshold_seconds))
    current_time=$(date +%s)

    # Check whether the current time is close to expiry
    if [ "$current_time" -ge "$expiry" ]; then
      echo 1
    else
      echo 0
    fi
  else
    echo 1
  fi
}

function call_api() {
  sub_path=$1
  shift 1

  url="$base_url/$sub_path"
  response="$(curl -s "$url" \
    -H 'Accept: application/json' \
    -H 'Content-Type: application/json' \
    -H "Authorization:Bearer $(cat $auth_token)" \
    -w '\nhttp_code=%{http_code}' \
    "$@")"

  # Request a new auth token if the current one hasn't expired yet
  get_auth_token

  response_pattern='^(.+?)\s*http_code=([0-9]+)$'
  if [[ $response =~ $response_pattern ]]; then
    echo "${BASH_REMATCH[1]}"
    http_response_code=${BASH_REMATCH[2]}
    if [[ $http_response_code -ne 200 ]]; then
      log "[ERROR] Request to $url failed (HTTP $http_response_code)"
      exit 1
    fi
  else
    log "[ERROR] Invalid HTTP request to: $url. Response: $response"
    exit 1
  fi
}