#!/bin/bash

stroom_root="${STROOM_SCRIPT_ROOT:-/stroom}"
api_log_file="$stroom_root/logs/k8s/api.log"
auth_token_file="$stroom_root/auth/token"
token_expiry_threshold_seconds=10
base_url="http://localhost:${STROOM_APP_HTTP_PORT}/api"
token_request_path='authproxy/v1/fetchClientCredsToken'
http_response_pattern='^(.+?)\s*http_code=([0-9]+)$'

#
# Logs a message to the specified log file, or to stderr if the file can't be written, which Kubernetes records in
# the probe or hook's failure event
#
function log() {
  log_msg=$1
  log_file=$2
  shift 2

  local line
  line="[$(date '+%Y-%m-%d %H:%M:%S')] $log_msg"
  if ! { echo "$line" >> "$log_file"; } 2> /dev/null; then
    echo "$line (unable to write to $log_file)" >&2
  fi
}

#
# Creates the directory the scripts log to. When the scripts run as root, it's given to the stroom user, which the
# Stroom image runs Stroom as, so that the logs can still be written if the pod is changed to run as that user.
#
function prepare_log_dir() {
  local log_dir="$stroom_root/logs/k8s"
  mkdir -p "$log_dir"
  if [ "$(id -u)" = '0' ] && id stroom > /dev/null 2>&1; then
    chown -R stroom:stroom "$log_dir"
  fi
}

#
# Requests an OAuth2 token using the Stroom auth token proxy endpoint
#
function get_auth_token() {
  if [ "$(is_token_expired)" -eq "1" ]; then
    url="$base_url/$token_request_path"
    # Read the client credentials from the environment and send them via stdin, so the secret doesn't appear in any
    # process' arguments
    response=$(jq -n '{ clientId: env.STROOM_OPERATOR_OPENID_CLIENT_ID, clientSecret: env.STROOM_OPERATOR_OPENID_CLIENT_SECRET }' |
      curl -s "$url" \
      -X POST --data-binary @- \
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

#
# Enable or disable Stroom processing jobs for the specified node
#
function set_job_status() {
  node_name=$1
  enabled=$2
  shift 2

  # Change the state of the provided job name list. If none provided, change the state of all jobs.
  managed_jobs=$(jq -Rnc --arg s "${STROOM_NODE_MANAGED_JOBS}" '$s | split(",")')
  call_api job/v1/setJobsEnabled/"$node_name" -X PUT -d "{ \"enabled\": $enabled, \"includeJobs\": $managed_jobs }"
}