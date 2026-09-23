# BOT_MAX - Patient Cabinet Makefile

.PHONY: run build clean status stop help

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

# Справка
help:
	@echo "Доступные команды:"
	@echo "  make run     - запустить сервер"
	@echo "  make build   - скомпилировать"
	@echo "  make stop    - остановить сервер"
	@echo "  make status  - проверить статус"
	@echo "  make test    - проверить работу"
	@echo "  make clean   - очистить"
