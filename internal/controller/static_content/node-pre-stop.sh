#!/bin/bash
# Executed by the pod pre-stop hook.

source "${STROOM_SCRIPT_ROOT:-/stroom}/scripts/utils.sh"

log_file="$stroom_root/logs/k8s/pre-stop.log"

prepare_log_dir

# Disable all node tasks, so the node can drain
set_job_status "${STROOM_NODE}" 'false'
log "Node ${STROOM_NODE} jobs disabled" $log_file

# Disable the node so the cluster doesn't attempt to contact it while it's unresponsive
call_api node/v1/enabled/"${STROOM_NODE}" -X PUT -d false
log "Node ${STROOM_NODE} disabled" $log_file

task_count=-1
while :
do
  # Get the number of active tasks for this node. If the request fails, retry rather than assuming the node has drained.
  task_count=''
  if task_list=$(call_api task/v1/list/"${STROOM_NODE}"); then
    task_count=$(echo "$task_list" | jq -r '.pageResponse.total // empty' 2> /dev/null)
  fi
  if [[ ! $task_count =~ ^[0-9]+$ ]]; then
    log "[ERROR] Unable to determine the number of active tasks for node ${STROOM_NODE}. Retrying." $log_file
  elif [[ $task_count -eq 0 ]]; then
    # Allow the node to shut down as there are no active tasks
    log "All tasks drained for node ${STROOM_NODE}. Node shutting down." $log_file
    exit 0
  else
    log "Shutdown blocked for node ${STROOM_NODE}, as there are still $task_count active tasks" $log_file
  fi

  sleep "${STROOM_DRAIN_POLL_SECONDS:-5}"
done
