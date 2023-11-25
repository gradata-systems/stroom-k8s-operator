#!/bin/bash
# Executed by the pod pre-stop hook.

source /stroom/scripts/utils.sh

log_file='/stroom/logs/k8s/pre-stop.log'

function log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> $log_file
}

mkdir -p "$(dirname $log_file)"

# Disable all node tasks, so the node can drain
call_api job/v1/setJobsEnabled/"${STROOM_NODE}" -X PUT -d '{ "enabled": false }'
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
