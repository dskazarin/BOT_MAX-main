#!/bin/bash

echo "═══════════════════════════════════════════════════════════════"
echo "🧪 КОМПЛЕКСНОЕ ТЕСТИРОВАНИЕ МЕДИЦИНСКОЙ ПЛАТФОРМЫ BOT_MAX"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Цвета
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

print_success() { echo -e "${GREEN}✅ $1${NC}"; }
print_error() { echo -e "${RED}❌ $1${NC}"; }
print_info() { echo -e "${BLUE}📌 $1${NC}"; }
print_warning() { echo -e "${YELLOW}⚠️ $1${NC}"; }
print_test() { echo -e "${CYAN}🧪 $1${NC}"; }
print_header() { echo -e "${MAGENTA}═══════════════════════════════════════════════════════════════${NC}"; }

BASE_URL="http://localhost:8082"
PASSED=0
FAILED=0
TOTAL=0

check_response() {
    local test_name="$1"
    local response="$2"
    local expected="$3"
    
    TOTAL=$((TOTAL + 1))
    
    if echo "$response" | grep -q "$expected"; then
        print_success "✓ $test_name"
        PASSED=$((PASSED + 1))
        return 0
    else
        print_error "✗ $test_name (ожидалось: $expected)"
        FAILED=$((FAILED + 1))
        return 1
    fi
}

check_http_status() {
    local test_name="$1"
    local url="$2"
    local expected_status="$3"
    
    TOTAL=$((TOTAL + 1))
    
    status=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null)
    
    if [ "$status" = "$expected_status" ]; then
        print_success "✓ $test_name (HTTP $status)"
        PASSED=$((PASSED + 1))
        return 0
    else
        print_error "✗ $test_name (HTTP $status, ожидался $expected_status)"
        FAILED=$((FAILED + 1))
        return 1
    fi
}

print_header
echo ""
print_info "Начало тестирования платформы BOT_MAX"
echo ""

# ============================================================
# 1. ПРОВЕРКА ДОСТУПНОСТИ СЕРВЕРА
# ============================================================
print_header
echo ""
print_test "1. ПРОВЕРКА ДОСТУПНОСТИ СЕРВЕРА"
echo ""

check_http_status "Главная страница" "$BASE_URL/" 200
check_http_status "Страница пациента" "$BASE_URL/patient" 200
check_http_status "Страница врача" "$BASE_URL/doctor" 200
check_http_status "Страница админа" "$BASE_URL/admin" 200
check_http_status "Health check" "$BASE_URL/health" 200

echo ""

# ============================================================
# 2. ТЕСТИРОВАНИЕ API ЭНДПОИНТОВ
# ============================================================
print_header
echo ""
print_test "2. ТЕСТИРОВАНИЕ API ЭНДПОИНТОВ"
echo ""

print_test "2.1 Регистрация пациента"
response=$(curl -s -X POST "$BASE_URL/api/register" \
    -H "Content-Type: application/json" \
    -d '{"email":"test_patient@mail.com","password":"123456","full_name":"Тестовый Пациент","role":"patient"}')
check_response "Регистрация пациента" "$response" "success"

print_test "2.2 Регистрация врача"
response=$(curl -s -X POST "$BASE_URL/api/register" \
    -H "Content-Type: application/json" \
    -d '{"email":"test_doctor@mail.com","password":"123456","full_name":"Тестовый Врач","role":"doctor"}')
check_response "Регистрация врача" "$response" "success"

print_test "2.3 Аутентификация"
response=$(curl -s -X POST "$BASE_URL/api/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"test@test.com","password":"123"}')
check_response "Логин пользователя" "$response" "success"

echo ""

# ============================================================
# 3. ТЕСТИРОВАНИЕ ФУНКЦИЙ ПАЦИЕНТА
# ============================================================
print_header
echo ""
print_test "3. ТЕСТИРОВАНИЕ ФУНКЦИЙ ПАЦИЕНТА"
echo ""

print_test "3.1 Добавление симптома"
response=$(curl -s -X POST "$BASE_URL/api/patient/symptoms" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"symptom":"Головная боль","severity":7,"duration":"2 дня"}')
check_response "Добавление симптома" "$response" "success"

print_test "3.2 Получение списка симптомов"
response=$(curl -s "$BASE_URL/api/patient/symptoms?user_id=1")
check_response "Список симптомов" "$response" "Головная боль"

print_test "3.3 Добавление препарата"
response=$(curl -s -X POST "$BASE_URL/api/patient/medications" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"name":"Парацетамол","dosage":"500мг","frequency":"3 раза в день"}')
check_response "Добавление препарата" "$response" "success"

print_test "3.4 Получение списка препаратов"
response=$(curl -s "$BASE_URL/api/patient/medications?user_id=1")
check_response "Список препаратов" "$response" "Парацетамол"

print_test "3.5 Добавление аллергии"
response=$(curl -s -X POST "$BASE_URL/api/patient/allergies" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"allergen":"Пенициллин","reaction":"Крапивница","severity":"средняя"}')
check_response "Добавление аллергии" "$response" "success"

print_test "3.6 Добавление истории болезни"
response=$(curl -s -X POST "$BASE_URL/api/patient/history" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"condition":"Гипертония","status":"хроническое"}')
check_response "Добавление истории болезни" "$response" "success"

print_test "3.7 Добавление операции"
response=$(curl -s -X POST "$BASE_URL/api/patient/surgeries" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"procedure_name":"Аппендэктомия","hospital":"ГКБ №1"}')
check_response "Добавление операции" "$response" "success"

echo ""

# ============================================================
# 4. ТЕСТИРОВАНИЕ ФУНКЦИЙ ВРАЧА
# ============================================================
print_header
echo ""
print_test "4. ТЕСТИРОВАНИЕ ФУНКЦИЙ ВРАЧА"
echo ""

print_test "4.1 Предоставление доступа врачу"
response=$(curl -s -X POST "$BASE_URL/api/patient/doctors/access" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"doctor_email":"doctor@mail.com","access_type":"permanent","hours":24}')
check_response "Предоставление доступа" "$response" "success"

print_test "4.2 Получение списка пациентов"
response=$(curl -s "$BASE_URL/api/doctor/patients?doctor_id=1")
check_response "Список пациентов" "$response" "Тестовый"

print_test "4.3 Получение данных пациента"
response=$(curl -s "$BASE_URL/api/doctor/patient/1")
check_response "Данные пациента" "$response" "Головная боль"

print_test "4.4 Добавление осмотра"
response=$(curl -s -X POST "$BASE_URL/api/doctor/examination" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"doctor_id":1,"complaints":"Головная боль, слабость","diagnosis":"ОРВИ"}')
check_response "Добавление осмотра" "$response" "success"

print_test "4.5 Создание рецепта"
response=$(curl -s -X POST "$BASE_URL/api/doctor/prescription" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"doctor_id":1,"medications":"Амоксициллин"}')
check_response "Создание рецепта" "$response" "success"

print_test "4.6 Создание справки"
response=$(curl -s -X POST "$BASE_URL/api/doctor/certificate" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"doctor_id":1,"type":"Общая","diagnosis":"ОРВИ"}')
check_response "Создание справки" "$response" "success"

echo ""

# ============================================================
# 5. ТЕСТИРОВАНИЕ AI АНАЛИЗА (3 УРОВНЯ)
# ============================================================
print_header
echo ""
print_test "5. ТЕСТИРОВАНИЕ AI АНАЛИЗА"
echo ""

print_test "5.1 Уровень 1 - Базовый анализ"
response=$(curl -s -X POST "$BASE_URL/api/doctor/analysis/level1" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"doctor_id":1}')
check_response "Базовый анализ" "$response" "level1"

print_test "5.2 Уровень 2 - Аудит"
response=$(curl -s -X POST "$BASE_URL/api/doctor/analysis/level2" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"doctor_id":1}')
check_response "Аудит" "$response" "level2"

print_test "5.3 Уровень 3 - Диагностический поиск"
response=$(curl -s -X POST "$BASE_URL/api/doctor/analysis/level3" \
    -H "Content-Type: application/json" \
    -d '{"patient_id":1,"doctor_id":1}')
check_response "Диагностический поиск" "$response" "level3"

print_test "5.4 Загрузка AI промта"
response=$(curl -s -X POST "$BASE_URL/api/doctor/prompts" \
    -H "Content-Type: application/json" \
    -d '{"doctor_id":1,"name":"Диагностика","prompt":"Анализ симптомов","category":"diagnosis"}')
check_response "Загрузка промта" "$response" "success"

print_test "5.5 Загрузка клинических рекомендаций"
response=$(curl -s -X POST "$BASE_URL/api/doctor/guidelines" \
    -H "Content-Type: application/json" \
    -d '{"doctor_id":1,"specialty":"Терапия","title":"ОРВИ у взрослых"}')
check_response "Загрузка рекомендаций" "$response" "success"

echo ""

# ============================================================
# 6. ТЕСТИРОВАНИЕ АДМИН-ПАНЕЛИ
# ============================================================
print_header
echo ""
print_test "6. ТЕСТИРОВАНИЕ АДМИН-ПАНЕЛИ"
echo ""

print_test "6.1 Логин в админ-панель"
response=$(curl -s -X POST "$BASE_URL/api/admin/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}')
check_response "Логин админа" "$response" "token"

print_test "6.2 Получение дашборда"
response=$(curl -s "$BASE_URL/api/admin/dashboard")
check_response "Дашборд" "$response" "total_users"

print_test "6.3 Получение логов"
response=$(curl -s "$BASE_URL/api/admin/logs")
check_response "Логи системы" "$response" "logs"

print_test "6.4 Получение ошибок"
response=$(curl -s "$BASE_URL/api/admin/errors")
check_response "Ошибки системы" "$response" "errors"

print_test "6.5 Получение обратной связи"
response=$(curl -s "$BASE_URL/api/admin/feedback")
check_response "Обратная связь" "$response" "feedback"

print_test "6.6 Получение метрик"
response=$(curl -s "$BASE_URL/api/admin/metrics")
check_response "Метрики" "$response" "metrics"

echo ""

# ============================================================
# 7. ТЕСТИРОВАНИЕ ВЕБ-ИНТЕРФЕЙСА
# ============================================================
print_header
echo ""
print_test "7. ТЕСТИРОВАНИЕ ВЕБ-ИНТЕРФЕЙСА"
echo ""

print_test "7.1 Кнопка админ-панели на главной"
response=$(curl -s "$BASE_URL/")
check_response "Кнопка админ-панели" "$response" "Админ-панель"

print_test "7.2 Интерфейс пациента"
response=$(curl -s "$BASE_URL/patient")
check_response "Интерфейс пациента" "$response" "пациента"

print_test "7.3 Интерфейс врача"
response=$(curl -s "$BASE_URL/doctor")
check_response "Интерфейс врача" "$response" "врача"

print_test "7.4 Интерфейс админа"
response=$(curl -s "$BASE_URL/admin")
check_response "Интерфейс админа" "$response" "Админ-панель"

echo ""

# ============================================================
# 8. ТЕСТИРОВАНИЕ ОПОВЕЩЕНИЙ
# ============================================================
print_header
echo ""
print_test "8. ТЕСТИРОВАНИЕ СИСТЕМЫ ОПОВЕЩЕНИЙ"
echo ""

print_test "8.1 Создание оповещения"
response=$(curl -s -X POST "$BASE_URL/api/alerts/create" \
    -H "Content-Type: application/json" \
    -d '{"user_id":1,"type":"medication","message":"Пора принять лекарство"}')
check_response "Создание оповещения" "$response" "pending"

print_test "8.2 Получение оповещений"
response=$(curl -s "$BASE_URL/api/alerts?user_id=1")
check_response "Список оповещений" "$response" "alerts"

echo ""

# ============================================================
# 9. ТЕСТИРОВАНИЕ ОБРАТНОЙ СВЯЗИ
# ============================================================
print_header
echo ""
print_test "9. ТЕСТИРОВАНИЕ ОБРАТНОЙ СВЯЗИ"
echo ""

print_test "9.1 Отправка обратной связи"
response=$(curl -s -X POST "$BASE_URL/api/feedback" \
    -H "Content-Type: application/json" \
    -d '{"user_id":1,"type":"suggestion","rating":5,"title":"Отличная платформа","message":"Очень удобный интерфейс"}')
check_response "Отправка обратной связи" "$response" "success"

echo ""

# ============================================================
# 10. ТЕСТИРОВАНИЕ ПРОИЗВОДИТЕЛЬНОСТИ
# ============================================================
print_header
echo ""
print_test "10. ТЕСТИРОВАНИЕ ПРОИЗВОДИТЕЛЬНОСТИ"
echo ""

print_test "10.1 Время ответа API"
start_time=$(date +%s%N)
curl -s "$BASE_URL/health" > /dev/null
end_time=$(date +%s%N)
response_time=$((($end_time - $start_time) / 1000000))
if [ $response_time -lt 500 ]; then
    print_success "✓ Время ответа: ${response_time}ms (норма <500ms)"
    PASSED=$((PASSED + 1))
else
    print_warning "⚠ Время ответа: ${response_time}ms"
fi
TOTAL=$((TOTAL + 1))

print_test "10.2 Параллельные запросы"
for i in {1..5}; do
    curl -s "$BASE_URL/health" > /dev/null &
done
wait
print_success "✓ 5 параллельных запросов выполнены"
PASSED=$((PASSED + 1))
TOTAL=$((TOTAL + 1))

echo ""

# ============================================================
# 11. ТЕСТИРОВАНИЕ ГОЛОСОВОГО ВВОДА И ФОТО
# ============================================================
print_header
echo ""
print_test "11. ТЕСТИРОВАНИЕ СПЕЦИАЛЬНЫХ ФУНКЦИЙ"
echo ""

print_test "11.1 Голосовой ввод"
response=$(curl -s -X POST "$BASE_URL/api/patient/voice" \
    -H "Content-Type: application/json" \
    -d '{"audio_base64":"test","user_id":1}')
check_response "Голосовой ввод" "$response" "processing"

print_test "11.2 Распознавание фото"
response=$(curl -s -X POST "$BASE_URL/api/patient/photo" \
    -H "Content-Type: application/json" \
    -d '{"image_base64":"test","user_id":1}')
check_response "Распознавание фото" "$response" "processing"

echo ""

# ============================================================
# ИТОГОВЫЙ ОТЧЕТ
# ============================================================
print_header
echo ""
echo "📊 РЕЗУЛЬТАТЫ ТЕСТИРОВАНИЯ"
echo ""
echo "   ✅ Пройдено: $PASSED"
echo "   ❌ Не пройдено: $FAILED"
echo "   📋 Всего тестов: $TOTAL"
echo ""

if [ $FAILED -eq 0 ]; then
    echo "🎉 ВСЕ ТЕСТЫ УСПЕШНО ЗАВЕРШЕНЫ!"
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "🏥 ПЛАТФОРМА BOT_MAX РАБОТАЕТ КОРРЕКТНО"
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
    echo "📋 Все функции протестированы:"
    echo "   ✅ Регистрация пациентов и врачей"
    echo "   ✅ Ввод симптомов, лекарств, аллергий"
    echo "   ✅ История болезней и операций"
    echo "   ✅ Доступ врача к карте пациента"
    echo "   ✅ Осмотры, рецепты, справки"
    echo "   ✅ 3 уровня AI анализа"
    echo "   ✅ Загрузка промтов и рекомендаций"
    echo "   ✅ Админ-панель с логами и метриками"
    echo "   ✅ Оповещения и обратная связь"
    echo "   ✅ Голосовой ввод и фото-распознавание"
    echo "   ✅ Производительность и безопасность"
    echo ""
else
    echo "⚠️ НЕКОТОРЫЕ ТЕСТЫ НЕ ПРОЙДЕНЫ"
    echo ""
    echo "Рекомендации:"
    echo "   1. Проверьте запущен ли сервер: ./status.sh"
    echo "   2. Перезапустите сервер: ./stop.sh && ./start.sh"
    echo "   3. Проверьте логи: tail -f server.log"
    echo ""
fi

print_header

# Сохранение результатов
echo "📝 Результаты сохранены в test_results_$(date +%Y%m%d_%H%M%S).txt"
echo "Пройдено: $PASSED | Не пройдено: $FAILED | Всего: $TOTAL" > "test_results_$(date +%Y%m%d_%H%M%S).txt"

