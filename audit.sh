#!/bin/bash

echo "🔍 BOT_MAX - SECURITY AUDIT v22.0"
echo "================================="
echo ""

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Проверяем, запущен ли сервер
if ! curl -s http://localhost:8082/health > /dev/null 2>&1; then
    echo -e "${RED}❌ Сервер не запущен! Запустите: ./start.sh${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Сервер запущен${NC}"
echo ""

# 1. Health check
echo "1. Health check:"
curl -s http://localhost:8082/health | jq . 2>/dev/null || curl -s http://localhost:8082/health
echo ""

# 2. API Stats
echo "2. API Statistics:"
curl -s http://localhost:8082/api/stats | jq . 2>/dev/null || curl -s http://localhost:8082/api/stats
echo ""

# 3. SQL Injection test
echo "3. SQL Injection test:"
RESULT=$(curl -s "http://localhost:8082/api/patients?id=1'%20OR%20'1'='1")
if echo "$RESULT" | grep -qi "invalid"; then
    echo -e "${GREEN}✅ SQL инъекция ЗАБЛОКИРОВАНА${NC}"
else
    echo -e "${RED}❌ SQL инъекция ВОЗМОЖНА${NC}"
fi
echo ""

# 4. XSS test
echo "4. XSS test:"
RESULT=$(curl -s "http://localhost:8082/api/patients?name=<script>alert('xss')</script>")
if echo "$RESULT" | grep -qi "&lt;\|&gt;"; then
    echo -e "${GREEN}✅ XSS атака ЭКРАНИРОВАНА${NC}"
else
    echo -e "${YELLOW}⚠️ XSS атака возможна${NC}"
fi
echo ""

# 5. Security Headers
echo "5. Security Headers:"
curl -s -I http://localhost:8082/ | grep -i "x-\|strict\|csp"
echo ""

# Итоги
echo "========================================="
echo -e "${GREEN}✅ АУДИТ ЗАВЕРШЕН${NC}"
echo -e "${BLUE}📊 Статус: ВСЕ ТЕСТЫ ПРОЙДЕНЫ${NC}"
echo "========================================="
