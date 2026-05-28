#!/bin/bash
cd "$(dirname "$0")/.."
cd src
go run start_cabinet.go &
echo "✅ Сервер запущен на http://localhost:8082"
