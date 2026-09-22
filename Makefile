# BOT_MAX - Patient Cabinet Makefile

.PHONY: run build clean status stop help

# Запуск сервера
run:
	@echo "🚀 Запуск сервера..."
	@go run backend/main.go &

# Сборка бинарника
build:
	@echo "🔧 Компиляция..."
	@go build -o botmax backend/main.go

# Очистка
clean:
	@echo "🧹 Очистка..."
	@rm -f botmax
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
