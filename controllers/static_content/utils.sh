#!/bin/bash

api_log_file='/stroom/logs/k8s/api.log'
auth_token_file='/stroom/auth/token'
token_expiry_threshold_seconds=10
base_url="http://localhost:${STROOM_APP_PORT}/api"
token_request_path='authproxy/v1/noauth/fetchClientCredsToken'
http_response_pattern='^(.+?)\s*http_code=([0-9]+)$'

#
# Logs a message to the specified log file
#
function log() {
  log_msg=$1
  log_file=$2
  shift 2

  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $log_msg" >> "$log_file"
}

#
# Requests an OAuth2 token using the Stroom auth token proxy endpoint
#
function get_auth_token() {
  if [ "$(is_token_expired)" -eq "1" ]; then
    url="$base_url/$token_request_path"
    client_credentials=$(printf '{ "clientId": "%s", "clientSecret": "%s" }' "${STROOM_OPERATOR_OPENID_CLIENT_ID}" "${STROOM_OPERATOR_OPENID_CLIENT_SECRET}")
    response=$(curl -s "$url" \
      -X POST -d "$client_credentials" \
      -H 'Accept: text/plain' \
      -H 'Content-Type: application/json' \
      -w '\nhttp_code=%{http_code}')

    if [[ "$response" =~ $http_response_pattern ]]; then
      token="${BASH_REMATCH[1]}"
      http_response_code=${BASH_REMATCH[2]}
      if [[ $http_response_code -ne 200 ]]; then
        log "[ERROR] Token request to $url failed. Response code: $http_response_code. Response: $response" $api_log_file
        exit 1
      else
        # Token was successfully retrieved. Write it to file so it can be used as long as it remains valid.
        echo "$token" > "$auth_token_file"
        chmod 600 "$auth_token_file"
        return 0
      fi
    else
      log "[ERROR] Invalid token request to: $url. Response: $response" $api_log_file
      exit 1
    fi
  fi
}

#
# Decodes a JWT and checks whether it has expired
#
function is_token_expired() {
  if [ -s "$auth_token_file" ]; then
    token=$(cat "$auth_token_file")

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

#
# Invokes a Stroom API method
#
function call_api() {
  sub_path=$1
  shift 1

  # Request a new auth token if we don't currently have one or the current token has expired
  get_auth_token

  url="$base_url/$sub_path"
  response=$(curl -s "$url" \
    -H 'Accept: application/json' \
    -H 'Content-Type: application/json' \
    -H "Authorization:Bearer $(cat $auth_token_file)" \
    -w '\nhttp_code=%{http_code}' \
    "$@")

  if [[ "$response" =~ $http_response_pattern ]]; then
    echo "${BASH_REMATCH[1]}"
    http_response_code=${BASH_REMATCH[2]}
    if [[ $http_response_code -ne 200 ]]; then
      log "[ERROR] Request to $url failed. Response code: $http_response_code. Response: $response" $api_log_file
      exit 1
    fi
  else
    log "[ERROR] Invalid HTTP request to: $url. Response: $response" $api_log_file
    exit 1
  fi
}