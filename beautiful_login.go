package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "sync"
    "time"
    
    "github.com/gorilla/mux"
)

var (
    clinicName    = "Медицинский центр BOT_MAX"
    clinicAddress = "г. Москва, ул. Медицинская, д. 1"
    clinicPhone   = "+7 (495) 123-45-67"
)

type Patient struct {
    ID         string    `json:"id"`
    FullName   string    `json:"full_name"`
    BirthDate  time.Time `json:"birth_date"`
    Gender     string    `json:"gender"`
    Phone      string    `json:"phone"`
    Email      string    `json:"email"`
    CardNumber string    `json:"card_number"`
    Password   string    `json:"password"`
    CreatedAt  time.Time `json:"created_at"`
}

type Doctor struct {
    ID       string `json:"id"`
    Email    string `json:"email"`
    Password string `json:"password"`
    FullName string `json:"full_name"`
}

var (
    patients   = make(map[string]Patient)
    patientsMu sync.RWMutex
)

func init() {
    // Демо-пациент
    patients["PAT1700000001"] = Patient{
        ID:         "PAT1700000001",
        FullName:   "Демо Пациент",
        BirthDate:  time.Date(1990, 1, 15, 0, 0, 0, 0, time.UTC),
        Gender:     "Мужской",
        Phone:      "+7 (999) 123-45-67",
        Email:      "patient@demo.com",
        CardNumber: "MC1700000001",
        Password:   "patient123",
        CreatedAt:  time.Now(),
    }
}

func main() {
    r := mux.NewRouter()
    
    // Страницы
    r.HandleFunc("/", homePage)
    r.HandleFunc("/patient-login", patientLoginPage)
    r.HandleFunc("/patient-registration", patientRegistrationPage)
    r.HandleFunc("/patient-dashboard/{patientId}", patientDashboardPage)
    r.HandleFunc("/doctor-login", doctorLoginPage)
    r.HandleFunc("/doctor-dashboard/{doctorId}", doctorDashboardPage)
    r.HandleFunc("/medical-appointment/{patientId}", medicalAppointmentPage)
    r.HandleFunc("/admin/settings", adminSettingsPage)
    
    // API
    r.HandleFunc("/api/patients", createPatient).Methods("POST")
    r.HandleFunc("/api/patients/list", getPatientsList).Methods("GET")
    r.HandleFunc("/api/patients/login", patientLogin).Methods("POST")
    r.HandleFunc("/api/doctors/login", doctorLogin).Methods("POST")
    
    port := "8082"
    log.Printf("🚀 Сервер запущен на http://localhost:%s", port)
    log.Fatal(http.ListenAndServe(":"+port, r))
}

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
        .admin-btn{position:fixed;top:20px;right:20px;background:rgba(255,255,255,0.2);backdrop-filter:blur(10px);color:white;padding:10px 20px;border-radius:30px;text-decoration:none;z-index:1000}
        .admin-btn:hover{background:rgba(255,255,255,0.3)}
        .container{max-width:1200px;margin:0 auto;padding:40px 20px}
        .header{text-align:center;color:white;margin-bottom:60px}
        .header h1{font-size:48px;margin-bottom:20px}
        .header p{font-size:20px;opacity:0.9}
        .cards{display:grid;grid-template-columns:repeat(2,1fr);gap:40px;max-width:900px;margin:0 auto}
        .card{background:white;border-radius:24px;padding:40px;text-align:center;box-shadow:0 20px 60px rgba(0,0,0,0.15);transition:transform 0.3s;cursor:pointer}
        .card:hover{transform:translateY(-10px)}
        .card-icon{font-size:64px;margin-bottom:20px}
        .card h2{font-size:28px;margin-bottom:15px;color:#333}
        .card p{color:#666;margin-bottom:25px;line-height:1.5}
        .btn-group{display:flex;gap:15px;justify-content:center;margin-top:20px}
        .btn{display:inline-block;padding:12px 28px;border-radius:40px;text-decoration:none;font-weight:600;transition:all 0.3s;cursor:pointer}
        .btn-primary{background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none}
        .btn-primary:hover{transform:translateY(-2px);box-shadow:0 5px 20px rgba(102,126,234,0.4)}
        .btn-outline{background:transparent;border:2px solid #667eea;color:#667eea}
        .btn-outline:hover{background:#667eea;color:white}
        .features{background:#f8f9fa;padding:60px 20px;border-radius:20px;margin-top:60px}
        .features h2{text-align:center;font-size:32px;margin-bottom:40px;color:#333}
        .features-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:30px;max-width:1000px;margin:0 auto}
        .feature{text-align:center}
        .feature-icon{font-size:40px;margin-bottom:15px}
        .feature h4{font-size:18px;margin-bottom:10px;color:#333}
        .feature p{color:#666;font-size:14px}
        .footer{text-align:center;padding:40px;color:white}
        @media (max-width:768px){.cards{grid-template-columns:1fr}.header h1{font-size:32px}}
    </style>
</head>
<body>
    <a href="/admin/settings" class="admin-btn">⚙️ Админ-панель</a>
    <div class="container">
        <div class="header">
            <h1>🏥 BOT_MAX</h1>
            <p>Медицинская платформа с искусственным интеллектом</p>
        </div>
        <div class="cards">
            <div class="card">
                <div class="card-icon">👨‍⚕️</div>
                <h2>Пациент</h2>
                <p>Ведите дневник здоровья, получайте рекомендации на основе AI анализа</p>
                <div class="btn-group">
                    <a href="/patient-login" class="btn btn-primary">Войти</a>
                    <a href="/patient-registration" class="btn btn-outline">Регистрация</a>
                </div>
            </div>
            <div class="card">
                <div class="card-icon">👩‍⚕️</div>
                <h2>Врач</h2>
                <p>Управляйте пациентами, назначайте лечение, анализируйте историю болезней</p>
                <div class="btn-group">
                    <a href="/doctor-login" class="btn btn-primary">Войти</a>
                </div>
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

func patientLoginPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Вход для пациента</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px;text-align:center}
        .header a{color:white;text-decoration:none;margin-left:20px}
        .container{max-width:450px;margin:0 auto;padding:40px}
        .form-card{background:white;border-radius:24px;padding:40px;box-shadow:0 20px 60px rgba(0,0,0,0.1)}
        .form-card h2{text-align:center;margin-bottom:30px;color:#333}
        .form-group{margin-bottom:25px}
        label{display:block;margin-bottom:8px;font-weight:600;color:#333}
        input{width:100%;padding:14px;border:2px solid #e0e0e0;border-radius:12px;font-size:15px;transition:all 0.3s}
        input:focus{outline:none;border-color:#667eea;box-shadow:0 0 0 3px rgba(102,126,234,0.1)}
        button{width:100%;padding:14px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;border-radius:12px;font-size:16px;font-weight:600;cursor:pointer;transition:all 0.3s}
        button:hover{transform:translateY(-2px);box-shadow:0 5px 20px rgba(102,126,234,0.4)}
        .demo-info{background:#e8f5e9;padding:15px;border-radius:12px;margin-bottom:25px;font-size:13px;color:#2e7d32;text-align:center}
        .demo-info strong{display:block;margin-bottom:8px}
        .register-link{text-align:center;margin-top:25px}
        .register-link a{color:#667eea;text-decoration:none}
        .error-message{color:#dc3545;text-align:center;margin-top:15px;padding:10px;background:#ffebee;border-radius:8px;display:none}
    </style>
</head>
<body>
    <div class="header"><h2>👨‍⚕️ Вход для пациента</h2><a href="/">← На главную</a></div>
    <div class="container">
        <div class="form-card">
            <h2>🔐 Добро пожаловать</h2>
            <div class="demo-info">
                <strong>📋 Демо-доступ</strong>
                Email: patient@demo.com<br>
                Пароль: patient123
            </div>
            <form id="loginForm">
                <div class="form-group">
                    <label>📧 Email</label>
                    <input type="email" id="email" placeholder="patient@example.com" required>
                </div>
                <div class="form-group">
                    <label>🔐 Пароль</label>
                    <input type="password" id="password" placeholder="••••••" required>
                </div>
                <button type="submit">Войти в кабинет</button>
            </form>
            <div id="errorMsg" class="error-message"></div>
            <div class="register-link">
                Нет аккаунта? <a href="/patient-registration">Зарегистрироваться</a>
            </div>
        </div>
    </div>
    <script>
        document.getElementById('loginForm').onsubmit = async (e) => {
            e.preventDefault();
            const email = document.getElementById('email').value;
            const password = document.getElementById('password').value;
            const errorDiv = document.getElementById('errorMsg');
            
            errorDiv.style.display = 'none';
            
            try {
                const response = await fetch('/api/patients/login', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({login: email, password: password})
                });
                const data = await response.json();
                
                if(data.success){
                    window.location.href = '/patient-dashboard/' + data.patient_id;
                } else {
                    errorDiv.textContent = '❌ Неверный email или пароль';
                    errorDiv.style.display = 'block';
                }
            } catch(error){
                errorDiv.textContent = '❌ Ошибка соединения с сервером';
                errorDiv.style.display = 'block';
            }
        };
    </script>
</body>
</html>`)
}

func patientRegistrationPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Регистрация пациента</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px;text-align:center}
        .header a{color:white;text-decoration:none;margin-left:20px}
        .container{max-width:500px;margin:0 auto;padding:40px}
        .form-card{background:white;border-radius:24px;padding:40px;box-shadow:0 20px 60px rgba(0,0,0,0.1)}
        .form-card h2{text-align:center;margin-bottom:30px;color:#333}
        .form-group{margin-bottom:20px}
        label{display:block;margin-bottom:8px;font-weight:600;color:#333}
        input,select{width:100%;padding:12px;border:2px solid #e0e0e0;border-radius:10px;font-size:14px}
        input:focus,select:focus{outline:none;border-color:#667eea}
        button{width:100%;padding:14px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;border-radius:12px;font-size:16px;font-weight:600;cursor:pointer;margin-top:10px}
        button:hover{transform:translateY(-2px);box-shadow:0 5px 20px rgba(102,126,234,0.4)}
        .login-link{text-align:center;margin-top:20px}
        .login-link a{color:#667eea;text-decoration:none}
    </style>
</head>
<body>
    <div class="header"><h2>📝 Регистрация пациента</h2><a href="/">← На главную</a></div>
    <div class="container">
        <div class="form-card">
            <h2>📋 Создание медицинской карты</h2>
            <form id="regForm">
                <div class="form-group"><label>ФИО *</label><input type="text" id="fullName" required placeholder="Иванов Иван Иванович"></div>
                <div class="form-group"><label>Дата рождения *</label><input type="date" id="birthDate" required></div>
                <div class="form-group"><label>Пол</label><select id="gender"><option>Мужской</option><option>Женский</option></select></div>
                <div class="form-group"><label>Телефон</label><input type="tel" id="phone" placeholder="+7 (999) 123-45-67"></div>
                <div class="form-group"><label>Email *</label><input type="email" id="email" required placeholder="patient@example.com"></div>
                <div class="form-group"><label>Пароль *</label><input type="password" id="password" required placeholder="придумайте пароль"></div>
                <button type="button" onclick="register()">Зарегистрироваться</button>
            </form>
            <div class="login-link">Уже есть аккаунт? <a href="/patient-login">Войти</a></div>
        </div>
    </div>
    <script>
        async function register(){
            const data={
                full_name: document.getElementById('fullName').value,
                birth_date: document.getElementById('birthDate').value,
                gender: document.getElementById('gender').value,
                phone: document.getElementById('phone').value,
                email: document.getElementById('email').value,
                password: document.getElementById('password').value
            };
            const res = await fetch('/api/patients', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(data)
            });
            const result = await res.json();
            if(result.success){
                alert('✅ Регистрация успешна!\n\nEmail: ' + data.email + '\nПароль: ' + data.password);
                window.location.href = '/patient-login';
            } else {
                alert('❌ Ошибка: ' + (result.error || 'Попробуйте другой email'));
            }
        }
    </script>
</body>
</html>`)
}

func patientDashboardPage(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["patientId"]
    
    patientsMu.RLock()
    patient, exists := patients[patientId]
    patientsMu.RUnlock()
    
    if !exists {
        http.Redirect(w, r, "/patient-login", http.StatusSeeOther)
        return
    }
    
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, `<!DOCTYPE html>
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
        .container{max-width:600px;margin:0 auto;padding:40px}
        .card{background:white;border-radius:24px;padding:35px;box-shadow:0 10px 40px rgba(0,0,0,0.1)}
        .welcome{font-size:24px;margin-bottom:20px;color:#333}
        .patient-info{background:#f8f9fa;border-radius:16px;padding:20px;margin:20px 0}
        .info-row{display:flex;margin-bottom:12px}
        .info-label{width:120px;font-weight:600;color:#555}
        .info-value{flex:1;color:#333}
        .btn{display:inline-block;padding:12px 28px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border-radius:40px;text-decoration:none;font-weight:600;margin:10px 5px;transition:all 0.3s}
        .btn:hover{transform:translateY(-2px);box-shadow:0 5px 20px rgba(102,126,234,0.4)}
        .btn-green{background:#28a745}
        .btn-green:hover{box-shadow:0 5px 20px rgba(40,167,69,0.4)}
    </style>
</head>
<body>
    <div class="header">
        <h2>🏥 Личный кабинет пациента</h2>
        <a href="/">🚪 Выйти</a>
    </div>
    <div class="container">
        <div class="card">
            <div class="welcome">👋 Здравствуйте, %s!</div>
            <div class="patient-info">
                <div class="info-row"><div class="info-label">📋 Номер карты:</div><div class="info-value">%s</div></div>
                <div class="info-row"><div class="info-label">📧 Email:</div><div class="info-value">%s</div></div>
                <div class="info-row"><div class="info-label">📅 Дата рождения:</div><div class="info-value">%s</div></div>
            </div>
            <a href="/medical-appointment/%s" class="btn">📋 Заполнить медицинскую карту</a>
            <a href="/" class="btn btn-green">🏠 На главную</a>
        </div>
    </div>
</body>
</html>`, patient.FullName, patient.CardNumber, patient.Email, patient.BirthDate.Format("02.01.2006"), patientId)
}

func doctorLoginPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Вход для врача</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px;text-align:center}
        .header a{color:white;text-decoration:none;margin-left:20px}
        .container{max-width:450px;margin:0 auto;padding:40px}
        .form-card{background:white;border-radius:24px;padding:40px;box-shadow:0 20px 60px rgba(0,0,0,0.1)}
        .form-card h2{text-align:center;margin-bottom:30px;color:#333}
        .form-group{margin-bottom:25px}
        label{display:block;margin-bottom:8px;font-weight:600;color:#333}
        input{width:100%;padding:14px;border:2px solid #e0e0e0;border-radius:12px;font-size:15px}
        input:focus{outline:none;border-color:#667eea}
        button{width:100%;padding:14px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;border-radius:12px;font-size:16px;font-weight:600;cursor:pointer}
        button:hover{transform:translateY(-2px);box-shadow:0 5px 20px rgba(102,126,234,0.4)}
        .demo-info{background:#e8f5e9;padding:15px;border-radius:12px;margin-bottom:25px;font-size:13px;color:#2e7d32;text-align:center}
    </style>
</head>
<body>
    <div class="header"><h2>👩‍⚕️ Вход для врача</h2><a href="/">← На главную</a></div>
    <div class="container">
        <div class="form-card">
            <h2>🔐 Доступ к системе</h2>
            <div class="demo-info">
                <strong>📋 Демо-доступ</strong>
                Логин: doctor@clinic.ru<br>
                Пароль: doctor123
            </div>
            <form id="loginForm">
                <div class="form-group"><label>📧 Email</label><input type="email" id="email" placeholder="doctor@clinic.ru" required></div>
                <div class="form-group"><label>🔐 Пароль</label><input type="password" id="password" placeholder="••••••" required></div>
                <button type="submit">Войти в систему</button>
            </form>
        </div>
    </div>
    <script>
        document.getElementById('loginForm').onsubmit = async (e) => {
            e.preventDefault();
            const response = await fetch('/api/doctors/login', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({email: document.getElementById('email').value, password: document.getElementById('password').value})
            });
            const data = await response.json();
            if(data.success) window.location.href = '/doctor-dashboard/' + data.doctor_id;
            else alert('❌ Ошибка входа');
        };
    </script>
</body>
</html>`)
}

func doctorDashboardPage(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    doctorId := vars["doctorId"]
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Кабинет врача</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px;display:flex;justify-content:space-between;align-items:center}
        .container{max-width:800px;margin:0 auto;padding:40px}
        .card{background:white;border-radius:24px;padding:35px;box-shadow:0 10px 40px rgba(0,0,0,0.1);text-align:center}
        .btn{display:inline-block;padding:12px 28px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border-radius:40px;text-decoration:none;margin-top:20px}
        .patient-list{margin-top:20px;text-align:left}
        .patient-item{padding:15px;border-bottom:1px solid #eee;cursor:pointer}
        .patient-item:hover{background:#f8f9fa}
    </style>
</head>
<body>
    <div class="header"><h2>👩‍⚕️ Кабинет врача</h2><a href="/">Выйти</a></div>
    <div class="container">
        <div class="card">
            <h2>👋 Добро пожаловать, доктор!</h2>
            <div class="patient-list"><h3>📋 Список пациентов</h3><div id="patientList">Загрузка...</div></div>
            <a href="/" class="btn">🏠 На главную</a>
        </div>
    </div>
    <script>
        async function loadPatients(){
            const res=await fetch('/api/patients/list');
            const patients=await res.json();
            const listDiv=document.getElementById('patientList');
            if(patients.length===0) listDiv.innerHTML='<div class="patient-item">Нет пациентов</div>';
            else listDiv.innerHTML=patients.map(p=>'<div class="patient-item" onclick="location.href=\'/medical-appointment/'+p.id+'\'"><strong>'+p.full_name+'</strong><br><small>📧 '+p.email+' | 📋 '+p.card_number+'</small></div>').join('');
        }
        loadPatients();
    </script>
</body>
</html>`, doctorId)
}

func medicalAppointmentPage(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["patientId"]
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Медицинский осмотр</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px;text-align:center}
        .container{max-width:800px;margin:0 auto;padding:20px}
        .card{background:white;border-radius:16px;padding:25px;margin-bottom:20px;box-shadow:0 2px 10px rgba(0,0,0,0.05)}
        .card h3{margin-bottom:15px;color:#667eea}
        textarea{width:100%;padding:12px;border:2px solid #e0e0e0;border-radius:10px;font-family:inherit;min-height:100px}
        button{background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;padding:14px;border-radius:10px;cursor:pointer;width:100%;font-size:16px;font-weight:600}
        .voice-btn{background:#28a745;width:auto;padding:10px 20px;margin-top:10px}
    </style>
</head>
<body>
    <div class="header"><h2>📋 Медицинский осмотр</h2><a href="/" style="color:white">← На главную</a></div>
    <div class="container">
        <div class="card"><h3>📝 Жалобы пациента</h3><textarea id="complaints" placeholder="Опишите жалобы..."></textarea><button class="voice-btn" onclick="startVoice('complaints')">🎤 Голосовой ввод</button></div>
        <div class="card"><h3>📋 Анамнез заболевания</h3><textarea id="diseaseHistory" placeholder="История заболевания..."></textarea><button class="voice-btn" onclick="startVoice('diseaseHistory')">🎤 Голосовой ввод</button></div>
        <button onclick="save()">💾 Сохранить медицинскую карту</button>
    </div>
    <script>
        function startVoice(fieldId){
            if('webkitSpeechRecognition' in window){
                const r=new webkitSpeechRecognition();
                r.lang='ru-RU';
                r.onresult=(e)=>{document.getElementById(fieldId).value+=e.results[0][0].transcript};
                r.start();
            }else alert('Голосовой ввод не поддерживается');
        }
        async function save(){
            await fetch('/api/appointments',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({patient_id:'` + patientId + `',complaints:document.getElementById('complaints').value,disease_history:document.getElementById('diseaseHistory').value})});
            alert('✅ Медицинская карта сохранена!');
            window.location.href='/patient-dashboard/` + patientId + `';
        }
    </script>
</body>
</html>`)
}

func adminSettingsPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><title>Настройки</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5;padding:40px}
.card{background:white;border-radius:24px;padding:40px;max-width:500px;margin:auto;box-shadow:0 20px 60px rgba(0,0,0,0.1)}
h2{margin-bottom:30px;color:#333}
input{width:100%;padding:12px;margin:10px 0;border:2px solid #e0e0e0;border-radius:10px}
button{width:100%;padding:14px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;border-radius:10px;cursor:pointer;margin-top:20px}
</style>
</head>
<body>
<div class="card"><h2>⚙️ Настройки клиники</h2>
<input type="text" id="name" placeholder="Название клиники" value="Медицинский центр BOT_MAX">
<input type="text" id="address" placeholder="Адрес" value="г. Москва, ул. Медицинская, д. 1">
<input type="text" id="phone" placeholder="Телефон" value="+7 (495) 123-45-67">
<button onclick="alert('✅ Настройки сохранены')">Сохранить</button>
<a href="/" style="display:block;text-align:center;margin-top:20px;color:#667eea">← На главную</a>
</div>
</body>
</html>`)
}

func createPatient(w http.ResponseWriter, r *http.Request) {
    var req struct {
        FullName  string `json:"full_name"`
        BirthDate string `json:"birth_date"`
        Gender    string `json:"gender"`
        Phone     string `json:"phone"`
        Email     string `json:"email"`
        Password  string `json:"password"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    id := fmt.Sprintf("PAT%d", time.Now().UnixNano())
    birthDate, _ := time.Parse("2006-01-02", req.BirthDate)
    cardNumber := fmt.Sprintf("MC%d", time.Now().Unix())
    
    patientsMu.Lock()
    patients[id] = Patient{
        ID: id, FullName: req.FullName, BirthDate: birthDate,
        Gender: req.Gender, Phone: req.Phone, Email: req.Email,
        CardNumber: cardNumber, Password: req.Password, CreatedAt: time.Now(),
    }
    patientsMu.Unlock()
    
    sendJSON(w, map[string]interface{}{"success": true, "patient_id": id, "card_number": cardNumber}, http.StatusOK)
}

func getPatientsList(w http.ResponseWriter, r *http.Request) {
    patientsMu.RLock()
    list := make([]Patient, 0, len(patients))
    for _, p := range patients {
        list = append(list, p)
    }
    patientsMu.RUnlock()
    sendJSON(w, list, http.StatusOK)
}

func patientLogin(w http.ResponseWriter, r *http.Request) {
    var req struct{ Login, Password string }
    json.NewDecoder(r.Body).Decode(&req)
    
    patientsMu.RLock()
    defer patientsMu.RUnlock()
    
    for _, p := range patients {
        if p.Email == req.Login && p.Password == req.Password {
            sendJSON(w, map[string]interface{}{"success": true, "patient_id": p.ID}, http.StatusOK)
            return
        }
    }
    sendJSON(w, map[string]interface{}{"success": false, "error": "Invalid credentials"}, http.StatusUnauthorized)
}

func doctorLogin(w http.ResponseWriter, r *http.Request) {
    var req struct{ Email, Password string }
    json.NewDecoder(r.Body).Decode(&req)
    
    if req.Email == "doctor@clinic.ru" && req.Password == "doctor123" {
        sendJSON(w, map[string]interface{}{"success": true, "doctor_id": "doc1"}, http.StatusOK)
        return
    }
    sendJSON(w, map[string]interface{}{"success": false, "error": "Invalid credentials"}, http.StatusUnauthorized)
}

func sendJSON(w http.ResponseWriter, data interface{}, status int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}
