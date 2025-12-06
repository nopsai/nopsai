#!/bin/bash

# --- Configuration ---
NUM_RUNS=10
NOPSAI_API_URL="http://localhost:8080"
WEBHOOK_URL="http://localhost:8081/webhook"
PAYLOAD_FILE="../doc/sample-git-event.json"
GITHUB_WEBHOOK_SECRET="vsfdverguhuyi3287467324ujfbsaihufb"
FIND_RUN_TIMEOUT=30
PIPELINE_RUN_TIMEOUT=600 # 10 minutes
# --- End Configuration ---

# Check for required tools
command -v openssl >/dev/null 2>&1 || { echo >&2 "Error: 'openssl' is required."; exit 1; }
command -v curl >/dev/null 2>&1 || { echo >&2 "Error: 'curl' is required."; exit 1; }
command -v jq >/dev/null 2>&1 || { echo >&2 "Error: 'jq' is required."; exit 1; }
command -v uuidgen >/dev/null 2>&1 || { echo >&2 "Error: 'uuidgen' is required."; exit 1; }
command -v awk >/dev/null 2>&1 || { echo >&2 "Error: 'awk' is required."; exit 1; }

# 1. Calculate the HMAC signature
SIGNATURE=$(openssl dgst -sha256 -hmac "$GITHUB_WEBHOOK_SECRET" "$PAYLOAD_FILE" | awk '{print $2}')
if [ -z "$SIGNATURE" ]; then
  echo >&2 "Error: Failed to calculate signature."
  exit 1
fi

# 2. Define the trigger and family-watch function
run_and_watch_family() {
  local run_index=$1
  local output_file=$2 # New argument for the results file
  local start_time=$(date +%s)
  local trigger_delay=0
  
  local delivery_id=$(uuidgen)
  
  echo "[Family $run_index] Firing trigger (ID: ${delivery_id:0:8}...)"
  
  # --- Trigger Phase ---
  local trigger_sent_time=$(date +%s)
  local status_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "X-GitHub-Event: push" \
    -H "X-GitHub-Delivery: $delivery_id" \
    -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
    --data-binary "@${PAYLOAD_FILE}" \
    "$WEBHOOK_URL")

  if [ "$status_code" != "200" ] && [ "$status_code" != "202" ]; then
    echo "[Family $run_index] ERROR: Trigger failed with HTTP status $status_code"
    return 1
  fi
  
  echo "[Family $run_index] Trigger sent. Watching for all runs..."

  # --- Watch Family Phase ---
  local run_start=$(date +%s)
  declare -a watched_runs=()
  declare -a completed_runs=()
  local all_runs_json=""
  local first_run_found_time=0

  while true; do
    all_runs_json=$(curl -s "${NOPSAI_API_URL}/v1/runs")
    if [ -z "$all_runs_json" ] || ! echo "$all_runs_json" | jq -e . > /dev/null 2>&1; then
        echo "[Family $run_index] warning: failed to fetch valid JSON from /v1/runs, retrying..."
        sleep 1
        continue
    fi

    local still_running=0
    local new_runs_found_this_loop=0

    # === 1. Discover NEW root runs (matching our trigger ID) ===
    local new_root_ids=$(echo "$all_runs_json" | jq -r --arg TRIG_ID "$delivery_id" '.[] | select(.trigger_event_id == $TRIG_ID) | .run_id')
    for new_root_id in $new_root_ids; do
        local is_watched=0
        for id in "${watched_runs[@]}"; do [ "$id" == "$new_root_id" ] && is_watched=1 && break; done
        
        if [ "$is_watched" -eq 0 ]; then
            echo "[Family $run_index] > Discovered root run: ${new_root_id:0:8}..."
            watched_runs+=("$new_root_id")
            new_runs_found_this_loop=1
            if [ "$first_run_found_time" -eq 0 ]; then
                first_run_found_time=$(date +%s)
                trigger_delay=$((first_run_found_time - trigger_sent_time))
                echo "[Family $run_index] > Trigger delay (webhook -> API): ${trigger_delay}s"
            fi
        fi
    done
    
    # === 2. Discover NEW child runs (from *any* run we are watching) ===
    if [ ${#watched_runs[@]} -gt 0 ]; then
        local parents_json=$(printf '%s\n' "${watched_runs[@]}" | jq -R . | jq -sc .)
        local new_children=$(echo "$all_runs_json" | jq -r --argjson PARENTS "$parents_json" '
          .[] | select(.parent_run_id != null) | . as $run | select(any($PARENTS[]; . == $run.parent_run_id)) | $run.run_id
        ')
        
        for new_child_id in $new_children; do
            local is_watched=0
            for id in "${watched_runs[@]}"; do [ "$id" == "$new_child_id" ] && is_watched=1 && break; done
            
            if [ "$is_watched" -eq 0 ]; then
                echo "[Family $run_index] > Discovered child run: ${new_child_id:0:8}..."
                watched_runs+=("$new_child_id")
                new_runs_found_this_loop=1
            fi
        done
    fi
    
    # === 3. Check status of all watched runs ===
    local temp_watched_for_status=("${watched_runs[@]}")
    for id_to_watch in "${temp_watched_for_status[@]}"; do
        local is_complete=0
        for completed_id in "${completed_runs[@]}"; do [ "$completed_id" == "$id_to_watch" ] && is_complete=1 && break; done
        if [ "$is_complete" -eq 1 ]; then continue; fi
        
        local status=$(echo "$all_runs_json" | jq -r --arg RUN_ID "$id_to_watch" '.[] | select(.run_id == $RUN_ID) | .status // "unknown"')
        
        if [ "$status" == "pending" ] || [ "$status" == "running" ]; then
            still_running=$((still_running + 1))
        elif [ "$status" != "unknown" ]; then
            echo "[Family $run_index] > Run ${id_to_watch:0:8}... finished with status: $status"
            completed_runs+=("$id_to_watch")
        else
            still_running=$((still_running + 1))
        fi
    done
    
    # === 4. Check Exit Conditions ===
    local now=$(date +%s)
    
    if [ "$still_running" -eq 0 ] && [ ${#watched_runs[@]} -gt 0 ] && [ ${#completed_runs[@]} -eq ${#watched_runs[@]} ]; then
        echo "[Family $run_index] All ${#watched_runs[@]} run(s) in family complete."
        break
    fi

    if [ ${#watched_runs[@]} -eq 0 ]; then
        if ((now - start_time > FIND_RUN_TIMEOUT)); then
            echo "[Family $run_index] ERROR: Timed out after $FIND_RUN_TIMEOUT seconds waiting for *any* run to appear."
            return 1
        fi
    fi

    if ((now - start_time > PIPELINE_RUN_TIMEOUT)); then
        echo "[Family $run_index] ERROR: Timed out after $PIPELINE_RUN_TIMEOUT seconds."
        return 1
    fi
    
    if [ "$new_runs_found_this_loop" -eq 1 ]; then
        echo "[Family $run_index] Status: ${still_running} running, ${#completed_runs[@]}/${#watched_runs[@]} complete. Watching family..."
    else
        echo "[Family $run_index] polling... (${still_running} running, ${#completed_runs[@]}/${#watched_runs[@]} complete)"
    fi
    
    sleep 5
  done

  # --- Report Phase ---
  printf "[Family $run_index] FINISHED. Reporting results to %s\n" "$output_file"
  local family_end_time=$(date +%s)
  local family_total_duration=$((family_end_time - start_time))
  
  # Fetch final details for all watched runs
  all_runs_json=$(curl -s "${NOPSAI_API_URL}/v1/runs")
  
  for id_to_report in "${watched_runs[@]}"; do
     echo "$all_runs_json" | jq -r --arg RUN_ID "$id_to_report" '
       .[] | select(.run_id == $RUN_ID) | 
       [.started_at, .finished_at, .status, .pipeline_name, .run_id, .parent_run_id] | @tsv
     ' | while IFS=$'\t' read -r start_ts end_ts status name run_id parent_id; do
     
        local duration_seconds=0
        if [ "$start_ts" != "null" ] && [ -n "$start_ts" ]; then
            local start_epoch=$(date -d "$start_ts" +%s 2>/dev/null || date -jf "%Y-%m-%dT%H:%M:%S" "$(echo $start_ts | cut -d. -f1)" +%s 2>/dev/null || echo 0)
            local end_epoch=$start_epoch
            if [ "$end_ts" != "null" ] && [ -n "$end_ts" ]; then
                end_epoch=$(date -d "$end_ts" +%s 2>/dev/null || date -jf "%Y-%m-%dT%H:%M:%S" "$(echo $end_ts | cut -d. -f1)" +%s 2>/dev/null || echo $start_epoch)
            fi
            # Ensure duration is at least 0
            [ "$end_epoch" -gt "$start_epoch" ] && duration_seconds=$((end_epoch - start_epoch))
        fi

        local relationship="ROOT"
        if [ "$parent_id" != "null" ] && [ -n "$parent_id" ]; then
            relationship="CHILD"
        fi
        
        # Write individual pipeline data to the temp file
        printf "PIPELINE %s %s %s %s %s %s\n" "$run_index" "$duration_seconds" "$name" "$run_id" "$status" "$relationship" >> "$output_file"
     done
  done
  
  # Write the total family duration and trigger delay to the temp file
  printf "FAMILY %s %s %s\n" "$run_index" "$family_total_duration" "$trigger_delay" >> "$output_file"
  echo "[Family $run_index] Total script time: ${family_total_duration}s"
}

# 3. Export the function and variables
export -f run_and_watch_family
export NOPSAI_API_URL WEBHOOK_URL PAYLOAD_FILE SIGNATURE FIND_RUN_TIMEOUT PIPELINE_RUN_TIMEOUT

# 4. Create a temporary directory for results
TMP_DIR=$(mktemp -d)
trap 'echo "Cleaning up temp directory $TMP_DIR"; rm -rf "$TMP_DIR"' EXIT

# 5. Start the test
echo "=============================================="
echo "Starting performance test: $NUM_RUNS concurrent triggers"
echo "Results will be stored in: $TMP_DIR"
echo "=============================================="

overall_start=$(date +%s)
pids=()

for (( i=1; i<=$NUM_RUNS; i++ )); do
  run_and_watch_family "$i" "$TMP_DIR/run_$i.log" &
  pids+=($!)
done

# 6. Wait for all background jobs (the watchers) to complete
echo "Waiting for all $NUM_RUNS run families to complete... (this may take several minutes)"
echo "You will see polling messages while the script waits."
wait ${pids[*]}

echo # Add a newline for cleaner output
overall_end=$(date +%s)
overall_duration=$((overall_end - overall_start))

# 7. Generate the final report
echo "=============================================="
echo "           PERFORMANCE TEST REPORT"
echo "=============================================="
echo
echo "--- Individual Family Reports ---"

for (( i=1; i<=$NUM_RUNS; i++ )); do
    FILE="$TMP_DIR/run_$i.log"
    if [ ! -f "$FILE" ]; then
        echo "WARNING: Result file for Run $i was not found."
        continue
    fi
    
    echo
    echo "------------------ [ Family $i ] ------------------"
    
    # Print Family stats
    grep "^FAMILY" "$FILE" | awk '{ 
        printf "  Total Family Duration: %s seconds\n", $3 
        printf "  Trigger->Run Delay:    %s seconds\n", $4 
    }'
    
    # Print Pipeline stats for this family
    echo "  Pipelines in this family:"
    grep "^PIPELINE" "$FILE" | awk '{ 
        # $1=PIPELINE, $2=run_index, $3=duration, $4=name, $5=run_id, $6=status, $7=relationship
        printf "    - ID: %-10s | Duration: %-5s | Status: %-10s | Name: %s (%s)\n", substr($5, 0, 8), $3"s", $6, $4, $7 
    }'
done

echo
echo "=============================================="
echo "           AGGREGATE SUMMARY"
echo "=============================================="

# Use AWK to process all files at once for the aggregate summary
cat $TMP_DIR/*.log | awk -v overall_duration=$overall_duration '
BEGIN {
    FS=" "
    total_pipeline_duration = 0
    pipeline_count = 0
    total_family_duration = 0
    family_count = 0
    total_trigger_delay = 0
    
    # For pretty printing
    blue = "\033[1;34m"
    green = "\033[1;32m"
    yellow = "\033[1;33m"
    reset = "\033[0m"
}
/^PIPELINE/ {
    total_pipeline_duration += $3
    pipeline_count++
}
/^FAMILY/ {
    total_family_duration += $3
    total_trigger_delay += $4
    family_count++
}
END {
    avg_pipeline = (pipeline_count > 0) ? total_pipeline_duration / pipeline_count : 0
    avg_family = (family_count > 0) ? total_family_duration / family_count : 0
    avg_delay = (family_count > 0) ? total_trigger_delay / family_count : 0
    
    printf "Total Families Triggered:   %d\n", family_count
    printf "Total Pipelines Executed: %d\n", pipeline_count
    printf "Overall Test Duration:      %d seconds (Wall clock time)\n", overall_duration
    
    print ""
    print blue "--- Pipeline Stats (All Runs) ---" reset
    printf "Total Duration:    %.f seconds (Sum of all individual run durations)\n", total_pipeline_duration
    printf "Average Duration:  %.2f seconds\n", avg_pipeline
    
    print ""
    print green "--- Family Stats (All Triggers) ---" reset
    printf "Total Duration:    %.f seconds (Sum of all family-watch times)\n", total_family_duration
    printf "Average Duration:  %.2f seconds (Avg. time from trigger to last child completion)\n", avg_family
    
    print ""
    print yellow "--- Trigger Stats ---" reset
    printf "Total Delay:       %.f seconds (Sum of all trigger->run-appear delays)\n", total_trigger_delay
    printf "Average Delay:     %.2f seconds (Avg. time from webhook sent to run in API)\n", avg_delay
    
    print "\n=============================================="
}
'