# BOT_MAX - Patient Cabinet Makefile

.PHONY: run build clean status stop help

# Запуск сервера
run:
	@echo "🚀 Запуск сервера..."
	@cd src && go run start_cabinet.go &

# Сборка бинарника
build:
	@echo "🔧 Компиляция..."
	@cd src && go build -o botmax start_cabinet.go

# Очистка
clean:
	@echo "🧹 Очистка..."
	@rm -f src/botmax
	@rm -f *.log
	@echo "✅ Готово"

# Статус сервера
status:
	@echo "📊 Статус сервера:"
	@if curl -s http://localhost:8082/ > /dev/null 2>&1; then \
		echo "✅ Сервер работает на http://localhost:8082"; \
	else \
		echo "❌ Сервер не запущен"; \
	fi

# Остановка сервера
stop:
	@echo "🛑 Остановка сервера..."
	@pkill -f "start_cabinet\|go run" 2>/dev/null || true
	@sudo fuser -k 8082/tcp 2>/dev/null || true
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
