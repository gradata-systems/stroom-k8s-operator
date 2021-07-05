#!/bin/bash

log_file='/stroom/logs/k8s/pre-stop.log'
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

# Disable all node tasks, so the node can drain
call_api node/v1/setJobsEnabled/"${STROOM_NODE}" -X PUT -d '{ "enabled": false }'
log "Node ${STROOM_NODE} jobs enabled"

# Disable the node so the cluster doesn't attempt to contact it while it's unresponsive
call_api node/v1/enabled/"${STROOM_NODE}" -X PUT -d false
log "Node ${STROOM_NODE} disabled"

task_count=-1
while :
do
  # Get the number of active tasks for this node
  task_list="$(call_api task/v1/list/"${STROOM_NODE}")"
  task_count=$(echo "$task_list" | jq '.pageResponse.total')
  if [[ $task_count -eq 0 ]]; then
    # Allow the node to shut down as there are no active tasks
    log "All tasks drained for node ${STROOM_NODE}. Node shutting down."
    exit 0
  else
    log "Shutdown blocked for node ${STROOM_NODE}, as there are still $task_count active tasks"
  fi

  sleep 5
done
