package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "time"
    
    "github.com/gorilla/mux"
)

func main() {
    r := mux.NewRouter()
    
    // Страницы
    r.HandleFunc("/", homePage)
    r.HandleFunc("/patient", patientPage)
    r.HandleFunc("/doctor", doctorPage)
    r.HandleFunc("/admin", adminPage)
    r.HandleFunc("/health", healthCheck)
    
    // API эндпоинты
    r.HandleFunc("/api/register", registerHandler).Methods("POST")
    r.HandleFunc("/api/login", loginHandler).Methods("POST")
    r.HandleFunc("/api/patient/symptoms", addSymptom).Methods("POST")
    r.HandleFunc("/api/patient/symptoms", getSymptoms).Methods("GET")
    r.HandleFunc("/api/patient/medications", addMedication).Methods("POST")
    r.HandleFunc("/api/patient/medications", getMedications).Methods("GET")
    r.HandleFunc("/api/patient/allergies", addAllergy).Methods("POST")
    r.HandleFunc("/api/patient/allergies", getAllergies).Methods("GET")
    r.HandleFunc("/api/patient/history", addMedicalHistory).Methods("POST")
    r.HandleFunc("/api/patient/history", getMedicalHistory).Methods("GET")
    r.HandleFunc("/api/patient/surgeries", addSurgery).Methods("POST")
    r.HandleFunc("/api/patient/surgeries", getSurgeries).Methods("GET")
    r.HandleFunc("/api/patient/voice", voiceHandler).Methods("POST")
    r.HandleFunc("/api/patient/photo", photoHandler).Methods("POST")
    r.HandleFunc("/api/patient/doctors/access", grantAccess).Methods("POST")
    r.HandleFunc("/api/doctor/patients", getPatients).Methods("GET")
    r.HandleFunc("/api/doctor/patient/{id}", getPatientData).Methods("GET")
    r.HandleFunc("/api/doctor/examination", addExamination).Methods("POST")
    r.HandleFunc("/api/doctor/prescription", createPrescription).Methods("POST")
    r.HandleFunc("/api/doctor/certificate", createCertificate).Methods("POST")
    r.HandleFunc("/api/doctor/analysis/level1", analysisLevel1).Methods("POST")
    r.HandleFunc("/api/doctor/analysis/level2", analysisLevel2).Methods("POST")
    r.HandleFunc("/api/doctor/analysis/level3", analysisLevel3).Methods("POST")
    r.HandleFunc("/api/doctor/prompts", uploadPrompt).Methods("POST")
    r.HandleFunc("/api/doctor/guidelines", uploadGuideline).Methods("POST")
    r.HandleFunc("/api/admin/login", adminLogin).Methods("POST")
    r.HandleFunc("/api/admin/dashboard", adminDashboard).Methods("GET")
    r.HandleFunc("/api/admin/logs", getLogs).Methods("GET")
    r.HandleFunc("/api/admin/errors", getErrors).Methods("GET")
    r.HandleFunc("/api/admin/feedback", getFeedback).Methods("GET")
    r.HandleFunc("/api/admin/metrics", getMetrics).Methods("GET")
    r.HandleFunc("/api/alerts", getAlerts).Methods("GET")
    r.HandleFunc("/api/alerts/create", createAlert).Methods("POST")
    r.HandleFunc("/api/feedback", submitFeedback).Methods("POST")
    
    port := "8082"
    log.Printf("🚀 Сервер запущен на http://localhost:%s", port)
    log.Fatal(http.ListenAndServe(":"+port, r))
}

func sendJSON(w http.ResponseWriter, data interface{}, status int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

// API обработчики
func registerHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "user_id": 1}, http.StatusOK)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "user_id": 1, "role": "patient"}, http.StatusOK)
}

func addSymptom(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getSymptoms(w http.ResponseWriter, r *http.Request) {
    symptoms := []map[string]interface{}{
        {"symptom": "Головная боль", "severity": 7, "duration": "2 дня"},
    }
    sendJSON(w, symptoms, http.StatusOK)
}

func addMedication(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getMedications(w http.ResponseWriter, r *http.Request) {
    meds := []map[string]interface{}{
        {"name": "Парацетамол", "dosage": "500мг"},
    }
    sendJSON(w, meds, http.StatusOK)
}

func addAllergy(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getAllergies(w http.ResponseWriter, r *http.Request) {
    allergies := []map[string]interface{}{
        {"allergen": "Пенициллин", "reaction": "Крапивница"},
    }
    sendJSON(w, allergies, http.StatusOK)
}

func addMedicalHistory(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getMedicalHistory(w http.ResponseWriter, r *http.Request) {
    history := []map[string]interface{}{
        {"condition": "Гипертония", "status": "хроническое"},
    }
    sendJSON(w, history, http.StatusOK)
}

func addSurgery(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getSurgeries(w http.ResponseWriter, r *http.Request) {
    surgeries := []map[string]interface{}{
        {"procedure_name": "Аппендэктомия", "hospital": "ГКБ №1"},
    }
    sendJSON(w, surgeries, http.StatusOK)
}

func voiceHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "text": "Головная боль", "status": "processing"}, http.StatusOK)
}

func photoHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "text": "Парацетамол", "status": "processing"}, http.StatusOK)
}

func grantAccess(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getPatients(w http.ResponseWriter, r *http.Request) {
    patients := []map[string]interface{}{
        {"full_name": "Тестовый Пациент", "birth_date": "1990-01-01"},
    }
    sendJSON(w, patients, http.StatusOK)
}

func getPatientData(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{
        "full_name": "Тестовый Пациент",
        "symptoms": []map[string]interface{}{{"symptom": "Головная боль"}},
    }, http.StatusOK)
}

func addExamination(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func createPrescription(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "number": "RX001"}, http.StatusOK)
}

func createCertificate(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "number": "SP001"}, http.StatusOK)
}

func analysisLevel1(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 1, "result": "Базовый анализ завершен"}, http.StatusOK)
}

func analysisLevel2(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 2, "result": "Аудит лечения завершен"}, http.StatusOK)
}

func analysisLevel3(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 3, "result": "Диагностический поиск завершен"}, http.StatusOK)
}

func uploadPrompt(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func uploadGuideline(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func adminLogin(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "token": "admin_token"}, http.StatusOK)
}

func adminDashboard(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{
        "total_users": 150,
        "errors_24h": 3,
        "total_feedback": 45,
        "avg_rating": 4.7,
    }, http.StatusOK)
}

func getLogs(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"logs": []map[string]interface{}{}}, http.StatusOK)
}

func getErrors(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"errors": []map[string]interface{}{}}, http.StatusOK)
}

func getFeedback(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"feedback": []map[string]interface{}{}}, http.StatusOK)
}

func getMetrics(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"metrics": []map[string]interface{}{}}, http.StatusOK)
}

func getAlerts(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"alerts": []map[string]interface{}{}}, http.StatusOK)
}

func createAlert(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"status": "pending"}, http.StatusOK)
}

func submitFeedback(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"status": "ok", "time": time.Now().Unix()}, http.StatusOK)
}

// HTML страницы
func homePage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>BOT_MAX - Медицинская платформа</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);min-height:100vh}
        .admin-btn{position:fixed;top:20px;right:20px;background:rgba(255,255,255,0.2);backdrop-filter:blur(10px);color:white;padding:10px 20px;border-radius:30px;text-decoration:none;font-weight:bold;z-index:1000;transition:all 0.3s}
        .admin-btn:hover{background:rgba(255,255,255,0.3);transform:translateY(-2px)}
        .container{max-width:1200px;margin:0 auto;padding:40px 20px}
        .header{text-align:center;color:white;margin-bottom:60px}
        .header h1{font-size:48px;margin-bottom:20px}
        .header p{font-size:20px;opacity:0.9}
        .cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:30px;margin-bottom:60px}
        .card{background:white;border-radius:20px;padding:40px;text-align:center;box-shadow:0 10px 40px rgba(0,0,0,0.1);transition:transform 0.3s}
        .card:hover{transform:translateY(-10px)}
        .card h2{font-size:28px;margin-bottom:15px;color:#333}
        .card p{color:#666;margin-bottom:25px;line-height:1.6}
        .btn{display:inline-block;padding:12px 30px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border-radius:30px;text-decoration:none;font-weight:bold;transition:all 0.3s}
        .btn:hover{transform:translateY(-2px);box-shadow:0 5px 20px rgba(102,126,234,0.4)}
        .features{background:#f8f9fa;padding:60px 20px;border-radius:20px;margin-bottom:40px}
        .features h2{text-align:center;font-size:32px;margin-bottom:40px;color:#333}
        .features-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:30px;max-width:1000px;margin:0 auto}
        .feature{text-align:center}
        .feature-icon{font-size:40px;margin-bottom:15px}
        .feature h4{font-size:18px;margin-bottom:10px;color:#333}
        .feature p{color:#666;font-size:14px}
        .footer{text-align:center;padding:40px;color:white}
        @media (max-width:768px){.header h1{font-size:32px}.cards{grid-template-columns:1fr}}
    </style>
</head>
<body>
    <a href="/admin" class="admin-btn">🔐 Админ-панель</a>
    <div class="container">
        <div class="header">
            <h1>🏥 BOT_MAX</h1>
            <p>Медицинская платформа с искусственным интеллектом</p>
        </div>
        <div class="cards">
            <div class="card">
                <h2>👨‍⚕️ Пациент</h2>
                <p>Вносите симптомы, отслеживайте лекарства, получайте рекомендации</p>
                <a href="/patient" class="btn">Войти как пациент</a>
            </div>
            <div class="card">
                <h2>👩‍⚕️ Врач</h2>
                <p>Управляйте пациентами, назначайте лечение, анализируйте историю</p>
                <a href="/doctor" class="btn">Войти как врач</a>
            </div>
        </div>
        <div class="features">
            <h2>Возможности платформы</h2>
            <div class="features-grid">
                <div class="feature"><div class="feature-icon">📝</div><h4>Ввод симптомов</h4><p>Голосом, текстом или фото</p></div>
                <div class="feature"><div class="feature-icon">💊</div><h4>Напоминания</h4><p>О приёме лекарств</p></div>
                <div class="feature"><div class="feature-icon">🤖</div><h4>AI анализ</h4><p>3 уровня диагностики</p></div>
                <div class="feature"><div class="feature-icon">📋</div><h4>Рецепты и справки</h4><p>Автоматическое формирование</p></div>
                <div class="feature"><div class="feature-icon">🔔</div><h4>Оповещения</h4><p>При ухудшении состояния</p></div>
                <div class="feature"><div class="feature-icon">🔐</div><h4>Безопасность</h4><p>Военный уровень защиты</p></div>
            </div>
        </div>
        <div class="footer">
            <p>© 2024 BOT_MAX. Все права защищены.</p>
        </div>
    </div>
</body>
</html>`)
}

func patientPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Личный кабинет пациента</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px;display:flex;justify-content:space-between;align-items:center}
        .header a{color:white;text-decoration:none}
        .container{max-width:800px;margin:0 auto;padding:20px}
        .card{background:white;border-radius:15px;padding:20px;margin-bottom:20px;box-shadow:0 2px 10px rgba(0,0,0,0.05)}
        .card h3{margin-bottom:15px;color:#333}
        button{background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;padding:10px 20px;border-radius:8px;cursor:pointer;margin-right:10px}
        .voice-btn{background:#28a745}
        .photo-btn{background:#17a2b8}
        input,textarea{width:100%;padding:10px;margin:10px 0;border:1px solid #ddd;border-radius:8px}
    </style>
</head>
<body>
    <div class="header">
        <h2>🏥 Личный кабинет пациента</h2>
        <a href="/">🚪 Выйти</a>
    </div>
    <div class="container">
        <div class="card">
            <h3>📝 Добавить симптом</h3>
            <button class="voice-btn" onclick="alert('🎤 Голосовой ввод активирован')">🎤 Голосовой ввод</button>
            <button class="photo-btn" onclick="alert('📸 Камера активирована')">📸 Загрузить фото</button>
            <input type="text" placeholder="Симптом (например: головная боль)">
            <input type="range" min="1" max="10" value="5">
            <textarea placeholder="Дополнительные заметки..." rows="3"></textarea>
            <button onclick="alert('✅ Симптом добавлен')">➕ Добавить симптом</button>
        </div>
        <div class="card">
            <h3>💊 Мои препараты</h3>
            <div style="padding:10px;background:#f8f9fa;border-radius:8px;margin-bottom:10px">Парацетамол 500мг - 3 раза в день</div>
            <div style="padding:10px;background:#f8f9fa;border-radius:8px">Амоксициллин 1000мг - 2 раза в день</div>
        </div>
        <div class="card">
            <h3>⚠️ Аллергии</h3>
            <div style="padding:10px;background:#fff3cd;border-radius:8px;color:#856404">Пенициллин - крапивница, отек</div>
        </div>
    </div>
</body>
</html>`)
}

func doctorPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Кабинет врача</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px;display:flex;justify-content:space-between;align-items:center}
        .header a{color:white;text-decoration:none}
        .container{max-width:1200px;margin:0 auto;padding:20px;display:grid;grid-template-columns:300px 1fr;gap:20px}
        .patients-list{background:white;border-radius:15px;padding:20px;box-shadow:0 2px 10px rgba(0,0,0,0.05)}
        .patients-list h3{margin-bottom:15px}
        .patient-item{padding:12px;border-bottom:1px solid #eee;cursor:pointer;transition:background 0.3s}
        .patient-item:hover{background:#f8f9fa}
        .content-area{background:white;border-radius:15px;padding:20px;box-shadow:0 2px 10px rgba(0,0,0,0.05)}
        button{background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;padding:10px 20px;border-radius:8px;cursor:pointer;margin:5px}
        .analysis-btn{background:#28a745}
        @media (max-width:768px){.container{grid-template-columns:1fr}}
    </style>
</head>
<body>
    <div class="header">
        <h2>👩‍⚕️ Кабинет врача</h2>
        <a href="/">🚪 Выйти</a>
    </div>
    <div class="container">
        <div class="patients-list">
            <h3>📋 Мои пациенты</h3>
            <div class="patient-item" onclick="alert('Карта пациента открыта')">
                <strong>Иванов Иван</strong><br>
                <small>45 лет, последний визит: 20.03.2024</small>
            </div>
            <div class="patient-item" onclick="alert('Карта пациента открыта')">
                <strong>Петрова Мария</strong><br>
                <small>38 лет, последний визит: 19.03.2024</small>
            </div>
        </div>
        <div class="content-area">
            <h3>🩺 Карта пациента</h3>
            <div style="margin:20px 0;padding:15px;background:#f8f9fa;border-radius:10px">
                <p><strong>👤 ФИО:</strong> Иванов Иван Иванович</p>
                <p><strong>📅 Дата рождения:</strong> 15.05.1979</p>
                <p><strong>🩸 Группа крови:</strong> A(II)+</p>
            </div>
            <button class="analysis-btn" onclick="alert('🧠 AI анализ выполнен')">🧠 AI анализ истории</button>
            <button onclick="alert('💊 Рецепт сформирован')">💊 Сформировать рецепт</button>
            <button onclick="alert('📄 Справка сформирована')">📄 Сформировать справку</button>
            <button onclick="alert('🩺 Осмотр сохранен')">💾 Сохранить осмотр</button>
        </div>
    </div>
</body>
</html>`)
}

func adminPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Админ-панель BOT_MAX</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif}
        .login-container{display:flex;justify-content:center;align-items:center;min-height:100vh;background:linear-gradient(135deg,#667eea,#764ba2)}
        .login-card{background:white;padding:40px;border-radius:20px;width:380px;box-shadow:0 20px 60px rgba(0,0,0,0.3)}
        .login-card h2{text-align:center;margin-bottom:30px;color:#333}
        .login-card input{width:100%;padding:12px;margin:10px 0;border:2px solid #e0e0e0;border-radius:10px;font-size:14px}
        .login-card input:focus{outline:none;border-color:#667eea}
        .login-card button{width:100%;padding:12px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;border-radius:10px;cursor:pointer;font-size:16px;font-weight:bold}
        .dashboard{display:none}
        .admin-header{background:#16213e;color:white;padding:20px;display:flex;justify-content:space-between;align-items:center}
        .admin-header button{background:rgba(255,255,255,0.2);color:white;border:none;padding:8px 16px;border-radius:8px;cursor:pointer}
        .stats-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:20px;padding:30px}
        .stat-card{background:#16213e;padding:25px;border-radius:15px;color:white}
        .stat-card h3{font-size:14px;opacity:0.8;margin-bottom:10px}
        .stat-card .value{font-size:36px;font-weight:bold}
    </style>
</head>
<body>
    <div id="login" class="login-container">
        <div class="login-card">
            <h2>🔐 Админ-панель</h2>
            <input type="text" id="username" placeholder="Логин">
            <input type="password" id="password" placeholder="Пароль">
            <button onclick="login()">Войти в систему</button>
        </div>
    </div>
    <div id="dashboard" class="dashboard">
        <div class="admin-header">
            <h2>🏥 Админ-панель BOT_MAX</h2>
            <button onclick="logout()">🚪 Выйти</button>
        </div>
        <div class="stats-grid" id="stats"></div>
    </div>
    <script>
        async function login() {
            var username = document.getElementById('username').value;
            var password = document.getElementById('password').value;
            
            if (username === 'admin' && password === 'admin123') {
                document.getElementById('login').style.display = 'none';
                document.getElementById('dashboard').style.display = 'block';
                
                var response = await fetch('/api/admin/dashboard');
                var data = await response.json();
                
                document.getElementById('stats').innerHTML = 
                    '<div class="stat-card"><h3>👥 Всего пользователей</h3><div class="value">' + data.total_users + '</div></div>' +
                    '<div class="stat-card"><h3>⚠️ Ошибок за 24ч</h3><div class="value">' + data.errors_24h + '</div></div>' +
                    '<div class="stat-card"><h3>💬 Отзывов</h3><div class="value">' + data.total_feedback + '</div></div>' +
                    '<div class="stat-card"><h3>⭐ Средний рейтинг</h3><div class="value">' + data.avg_rating + '</div></div>';
            } else {
                alert('❌ Неверный логин или пароль');
            }
        }
        
        function logout() {
            document.getElementById('login').style.display = 'flex';
            document.getElementById('dashboard').style.display = 'none';
        }
    </script>
</body>
</html>`)
}
