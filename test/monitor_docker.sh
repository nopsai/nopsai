#!/bin/bash
OUTPUT_FILE="docker_monitor_$(date +%s).log"
echo "Monitoring Docker to $OUTPUT_FILE"

# Log Docker Events in background
docker events --format '{{.Time}} {{.Type}} {{.Action}} {{.Actor.Attributes.name}}' >> "$OUTPUT_FILE" &
EVENTS_PID=$!

echo "Timestamp, Containers, Running, CPU_Load, Mem_Free" >> "$OUTPUT_FILE"

while true; do
    TS=$(date +%T)
    CONTAINER_COUNT=$(docker ps -q | wc -l)
    RUNNING_COUNT=$(docker ps -q --filter status=running | wc -l)
    LOAD=$(uptime | awk -F'load average:' '{ print $2 }' | cut -d, -f1)
    MEM=$(free -m | grep Mem | awk '{print $4}')
    
    echo "$TS, $CONTAINER_COUNT, $RUNNING_COUNT, $LOAD, $MEM" >> "$OUTPUT_FILE"
    
    # Snapshot of top container CPU usage
    echo "--- Top CPU Containers ($TS) ---" >> "$OUTPUT_FILE"
    docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" | head -n 6 >> "$OUTPUT_FILE"
    echo "-------------------------------" >> "$OUTPUT_FILE"

    sleep 1
done

trap "kill $EVENTS_PID" EXIT
