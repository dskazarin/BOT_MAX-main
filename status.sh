#!/bin/bash
if pgrep -f "main_fixed" >/dev/null; then
    echo "✅ Server is running"
else
    echo "❌ Server is not running"
fi
