# BOT_MAX - Patient Cabinet Makefile

.PHONY: run build clean status stop help watchdog watchdog-stop watchdog-status watchdog-log

# Сборка бинарника (в /tmp, чтобы не мусорить в репо)
build:
	@echo "🔧 Компиляция..."
	@go build -o /tmp/botmax_server ./backend

# Запуск сервера (через собранный бинарник — один процесс, легко убить)
run: build
	@echo "🚀 Запуск сервера..."
	@nohup /tmp/botmax_server > /tmp/botmax.log 2>&1 &
	@sleep 3
	@make --no-print-directory status

# Очистка
clean:
	@echo "🧹 Очистка..."
	@rm -f botmax /tmp/botmax_server /tmp/botmax.log /tmp/botmax_build.log
	@rm -f *.log
	@echo "✅ Готово"

# Статус сервера
status:
	@echo "📊 Статус сервера:"
	@for i in 1 2 3 4 5; do \
		if curl -sf http://localhost:8082/api/health > /dev/null 2>&1; then \
			echo "✅ Сервер работает на http://localhost:8082"; \
			exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "❌ Сервер не запущен"

# Остановка сервера
stop:
	@echo "🛑 Остановка сервера..."
	@fuser -k 8082/tcp 2>/dev/null || true
	@pkill -f "/tmp/botmax_server" 2>/dev/null || true
	@echo "✅ Сервер остановлен"

# Проверка работы
test:
	@echo "🔍 Проверка..."
	@curl -s http://localhost:8082/ | head -10

# Watchdog для Go-сервера
watchdog:
	@if [ -f /tmp/botmax_watchdog.pid ] && kill -0 $$(cat /tmp/botmax_watchdog.pid) 2>/dev/null; then \
		echo "⚠️  Watchdog уже запущен (PID: $$(cat /tmp/botmax_watchdog.pid))"; \
	else \
		nohup bash /workspaces/BOT_MAX-main/.devcontainer/watchdog.sh > /dev/null 2>&1 & \
		sleep 1; \
		if [ -f /tmp/botmax_watchdog.pid ]; then \
			echo "✅ Watchdog запущен (PID=$$(cat /tmp/botmax_watchdog.pid))"; \
		else \
			echo "✅ Watchdog запущен (PID файл ещё не готов, проверь: make watchdog-status)"; \
		fi; \
		echo "📋 Лог: tail -f /tmp/botmax_watchdog.log"; \
	fi

watchdog-stop:
	@if [ -f /tmp/botmax_watchdog.pid ]; then \
		PID=$$(cat /tmp/botmax_watchdog.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID 2>/dev/null; \
			sleep 1; \
			kill -9 $$PID 2>/dev/null || true; \
			rm -f /tmp/botmax_watchdog.pid; \
			echo "✅ Watchdog остановлен (PID=$$PID)"; \
		else \
			rm -f /tmp/botmax_watchdog.pid; \
			echo "ℹ️  Watchdog не запущен (stale pid file удалён)"; \
		fi \
	else \
		echo "ℹ️  Watchdog не запущен"; \
	fi

watchdog-status:
	@if [ -f /tmp/botmax_watchdog.pid ] && kill -0 $$(cat /tmp/botmax_watchdog.pid) 2>/dev/null; then \
		echo "✅ Watchdog работает (PID=$$(cat /tmp/botmax_watchdog.pid))"; \
		tail -5 /tmp/botmax_watchdog.log 2>/dev/null || echo "(лог пуст)"; \
	else \
		echo "❌ Watchdog не запущен"; \
	fi

watchdog-log:
	@tail -f /tmp/botmax_watchdog.log

# Справка
help:
	@echo "Доступные команды:"
	@echo "  make run     - запустить сервер"
	@echo "  make build   - скомпилировать"
	@echo "  make stop    - остановить сервер"
	@echo "  make status  - проверить статус"
	@echo "  make test    - проверить работу"
	@echo "  make clean   - очистить"
