#!/bin/bash
# .devcontainer/watchdog.sh - restart BOT_MAX server on crash
LOG=/tmp/botmax_watchdog.log
PIDFILE=/tmp/botmax_watchdog.pid
CHECK_INTERVAL=30
SERVER_BIN=/tmp/botmax_server
SERVER_LOG=/tmp/botmax.log
PROJECT=/workspaces/BOT_MAX-main

echo $$ > "$PIDFILE"
trap 'rm -f "$PIDFILE"' EXIT

echo "[$(date '+%Y-%m-%d %H:%M:%S')] watchdog: start (PID=$$, interval=${CHECK_INTERVAL}s)" >> "$LOG"

while true; do
    sleep "$CHECK_INTERVAL"
    if ! pgrep -f "$SERVER_BIN" > /dev/null; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] watchdog: server not found, restarting..." >> "$LOG"
        cd "$PROJECT" || { echo "[$(date '+%Y-%m-%d %H:%M:%S')] watchdog: cd $PROJECT failed" >> "$LOG"; continue; }
        nohup "$SERVER_BIN" >> "$SERVER_LOG" 2>&1 &
        NEWPID=$!
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] watchdog: restarted PID=$NEWPID" >> "$LOG"
    fi
done
