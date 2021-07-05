#!/bin/bash

log_file='/stroom/logs/k8s/node-start.log'
base_url="http://localhost:${STROOM_APP_PORT}/api"
http_response_code=0

function log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> $log_file
}

function call_api() {
  sub_path=$1
  shift 1

  url="$base_url/$sub_path"
  response="$(curl -s "$url" \
    -H 'Accept: application/json' \
    -H 'Content-Type: application/json' \
    -H "Authorization:Bearer $(cat "${API_KEY}")" \
    -w '\nhttp_code=%{http_code}' \
    "$@")"

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

mkdir -p "$(dirname $log_file)"

# Enable the node
call_api node/v1/enabled/"${STROOM_NODE}" -X PUT -d true
log "Node ${STROOM_NODE} enabled"

# Enable all node tasks (except if this is a dedicated UI nodes)
enabled='true'
if [[ "${STROOM_NODE_ROLE}" == 'Frontend' ]]; then
  enabled='false'
fi
call_api node/v1/setJobsEnabled/"${STROOM_NODE}" -X PUT -d "{ \"enabled\": $enabled }"
if [[ $enabled == 'true' ]]; then
  log "Node ${STROOM_NODE} jobs enabled"
else
  log "Node ${STROOM_NODE} jobs disabled (as this is a dedicated UI node)"
fi