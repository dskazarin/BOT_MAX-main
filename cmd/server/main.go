package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/gorilla/mux"
    "github.com/gorilla/websocket"
    "golang.org/x/crypto/bcrypt"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

var db *gorm.DB
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

// Модели данных
type User struct {
    ID        uint      `gorm:"primaryKey"`
    Email     string    `gorm:"unique"`
    Phone     string
    Password  string
    FullName  string
    Role      string    `gorm:"default:patient"`
    CreatedAt time.Time
}

type Symptom struct {
    ID         uint      `gorm:"primaryKey"`
    PatientID  uint
    Symptom    string
    Severity   int
    Duration   string
    Notes      string
    RecordedAt time.Time
}

type Medication struct {
    ID        uint      `gorm:"primaryKey"`
    PatientID uint
    Name      string
    Dosage    string
    Frequency string
    StartDate time.Time
    EndDate   time.Time
}

type Allergy struct {
    ID        uint   `gorm:"primaryKey"`
    PatientID uint
    Allergen  string
    Reaction  string
    Severity  string
}

type MedicalHistory struct {
    ID         uint      `gorm:"primaryKey"`
    PatientID  uint
    Condition  string
    DiagnosedAt time.Time
    Status     string
}

type Surgery struct {
    ID           uint      `gorm:"primaryKey"`
    PatientID    uint
    ProcedureName string
    PerformedAt  time.Time
    Hospital     string
}

type Examination struct {
    ID               uint      `gorm:"primaryKey"`
    PatientID        uint
    DoctorID         uint
    Complaints       string
    ObjectiveFindings string
    VitalSigns       string
    Diagnosis        string
    Recommendations  string
    ExaminedAt       time.Time
}

type Prescription struct {
    ID          uint      `gorm:"primaryKey"`
    PatientID   uint
    DoctorID    uint
    Number      string
    Medications string
    IssuedAt    time.Time
    ExpiresAt   time.Time
    Status      string
}

type Certificate struct {
    ID          uint      `gorm:"primaryKey"`
    PatientID   uint
    DoctorID    uint
    Number      string
    Type        string
    Diagnosis   string
    PeriodStart time.Time
    PeriodEnd   time.Time
    IssuedAt    time.Time
}

type AnalysisRequest struct {
    ID         uint      `gorm:"primaryKey"`
    PatientID  uint
    DoctorID   uint
    Level      int
    Status     string
    Result     string
    CreatedAt  time.Time
}

type AIPrompt struct {
    ID       uint   `gorm:"primaryKey"`
    DoctorID uint
    Name     string
    Prompt   string
    Category string
}

type ClinicalGuideline struct {
    ID          uint   `gorm:"primaryKey"`
    DoctorID    uint
    Specialty   string
    DiseaseCode string
    Title       string
    Content     string
}

type DoctorAccess struct {
    ID         uint      `gorm:"primaryKey"`
    PatientID  uint
    DoctorID   uint
    AccessType string
    ExpiresAt  time.Time
    CreatedAt  time.Time
}

type Alert struct {
    ID        uint      `gorm:"primaryKey"`
    UserID    uint
    Type      string
    Message   string
    IsRead    bool
    CreatedAt time.Time
}

type UserFeedback struct {
    ID        uint      `gorm:"primaryKey"`
    UserID    uint
    Type      string
    Rating    int
    Title     string
    Message   string
    Status    string
    CreatedAt time.Time
}

type SystemLog struct {
    ID        uint      `gorm:"primaryKey"`
    Level     string
    Component string
    Message   string
    CreatedAt time.Time
}

type SystemError struct {
    ID        uint      `gorm:"primaryKey"`
    Type      string
    Message   string
    Severity  string
    Status    string
    CreatedAt time.Time
}

type PerformanceMetric struct {
    ID        uint      `gorm:"primaryKey"`
    Name      string
    Value     float64
    Unit      string
    CreatedAt time.Time
}

type Admin struct {
    ID       uint   `gorm:"primaryKey"`
    Username string `gorm:"unique"`
    Email    string `gorm:"unique"`
    Password string
    Role     string
}

func main() {
    initDB()
    
    r := mux.NewRouter()
    
    // Статические файлы
    r.PathPrefix("/css/").Handler(http.StripPrefix("/css/", http.FileServer(http.Dir("./web/css"))))
    r.PathPrefix("/js/").Handler(http.StripPrefix("/js/", http.FileServer(http.Dir("./web/js"))))
    
    // Страницы
    r.HandleFunc("/", homePage)
    r.HandleFunc("/patient", patientPage)
    r.HandleFunc("/doctor", doctorPage)
    r.HandleFunc("/admin", adminPage)
    r.HandleFunc("/health", healthCheck)
    
    // API маршруты
    r.HandleFunc("/api/register", registerHandler).Methods("POST")
    r.HandleFunc("/api/login", loginHandler).Methods("POST")
    r.HandleFunc("/api/logout", logoutHandler).Methods("POST")
    
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
    r.HandleFunc("/api/patient/voice", voiceInput).Methods("POST")
    r.HandleFunc("/api/patient/photo", photoInput).Methods("POST")
    r.HandleFunc("/api/patient/doctors/access", grantDoctorAccess).Methods("POST")
    
    r.HandleFunc("/api/doctor/patients", getMyPatients).Methods("GET")
    r.HandleFunc("/api/doctor/patient/{id}", getPatientData).Methods("GET")
    r.HandleFunc("/api/doctor/examination", addExamination).Methods("POST")
    r.HandleFunc("/api/doctor/prescription", createPrescription).Methods("POST")
    r.HandleFunc("/api/doctor/certificate", createCertificate).Methods("POST")
    r.HandleFunc("/api/doctor/analysis/level1", requestLevel1Analysis).Methods("POST")
    r.HandleFunc("/api/doctor/analysis/level2", requestLevel2Analysis).Methods("POST")
    r.HandleFunc("/api/doctor/analysis/level3", requestLevel3Analysis).Methods("POST")
    r.HandleFunc("/api/doctor/prompts", uploadPrompt).Methods("POST")
    r.HandleFunc("/api/doctor/guidelines", uploadGuideline).Methods("POST")
    
    r.HandleFunc("/api/admin/login", adminLogin).Methods("POST")
    r.HandleFunc("/api/admin/dashboard", adminDashboard).Methods("GET")
    r.HandleFunc("/api/admin/logs", getLogs).Methods("GET")
    r.HandleFunc("/api/admin/errors", getErrors).Methods("GET")
    r.HandleFunc("/api/admin/feedback", getFeedback).Methods("GET")
    r.HandleFunc("/api/admin/metrics", getMetrics).Methods("GET")
    
    r.HandleFunc("/api/alerts/create", createAlert).Methods("POST")
    r.HandleFunc("/api/alerts", getAlerts).Methods("GET")
    r.HandleFunc("/api/feedback", submitFeedback).Methods("POST")
    
    r.HandleFunc("/ws", websocketHandler)
    
    port := os.Getenv("PORT")
    if port == "" {
        port = "8082"
    }
    
    srv := &http.Server{
        Addr:         ":" + port,
        Handler:      r,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
    }
    
    go func() {
        log.Printf("🚀 Сервер запущен на порту %s", port)
        log.Printf("🌐 Откройте: http://localhost:%s", port)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal(err)
        }
    }()
    
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    log.Println("🛑 Завершение работы...")
    srv.Close()
}

func initDB() {
    var err error
    db, err = gorm.Open(sqlite.Open("medical.db"), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        log.Fatal("Ошибка подключения к БД:", err)
    }
    
    db.AutoMigrate(&User{}, &Symptom{}, &Medication{}, &Allergy{}, &MedicalHistory{}, &Surgery{},
        &Examination{}, &Prescription{}, &Certificate{}, &AnalysisRequest{}, &AIPrompt{},
        &ClinicalGuideline{}, &DoctorAccess{}, &Alert{}, &UserFeedback{}, &SystemLog{},
        &SystemError{}, &PerformanceMetric{}, &Admin{})
    
    var admin Admin
    if db.Where("username = ?", "admin").First(&admin).Error != nil {
        hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
        db.Create(&Admin{
            Username: "admin",
            Email:    "admin@botmax.com",
            Password: string(hashedPassword),
            Role:     "super_admin",
        })
        log.Println("✅ Администратор создан: admin / admin123")
    }
}

func sendJSON(w http.ResponseWriter, data interface{}, status int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

// API Обработчики
func registerHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email    string `json:"email"`
        Phone    string `json:"phone"`
        Password string `json:"password"`
        FullName string `json:"full_name"`
        Role     string `json:"role"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    user := User{
        Email:     req.Email,
        Phone:     req.Phone,
        Password:  string(hashedPassword),
        FullName:  req.FullName,
        Role:      req.Role,
        CreatedAt: time.Now(),
    }
    db.Create(&user)
    sendJSON(w, map[string]interface{}{"success": true, "user_id": user.ID}, http.StatusOK)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email    string `json:"email"`
        Password string `json:"password"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    var user User
    if db.Where("email = ?", req.Email).First(&user).Error != nil {
        sendJSON(w, map[string]interface{}{"success": false, "error": "User not found"}, http.StatusUnauthorized)
        return
    }
    
    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
        sendJSON(w, map[string]interface{}{"success": false, "error": "Invalid password"}, http.StatusUnauthorized)
        return
    }
    
    sendJSON(w, map[string]interface{}{"success": true, "user_id": user.ID, "role": user.Role}, http.StatusOK)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func addSymptom(w http.ResponseWriter, r *http.Request) {
    var req Symptom
    json.NewDecoder(r.Body).Decode(&req)
    req.RecordedAt = time.Now()
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getSymptoms(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    var symptoms []Symptom
    db.Where("patient_id = ?", userID).Find(&symptoms)
    sendJSON(w, symptoms, http.StatusOK)
}

func addMedication(w http.ResponseWriter, r *http.Request) {
    var req Medication
    json.NewDecoder(r.Body).Decode(&req)
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getMedications(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    var medications []Medication
    db.Where("patient_id = ?", userID).Find(&medications)
    sendJSON(w, medications, http.StatusOK)
}

func addAllergy(w http.ResponseWriter, r *http.Request) {
    var req Allergy
    json.NewDecoder(r.Body).Decode(&req)
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getAllergies(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    var allergies []Allergy
    db.Where("patient_id = ?", userID).Find(&allergies)
    sendJSON(w, allergies, http.StatusOK)
}

func addMedicalHistory(w http.ResponseWriter, r *http.Request) {
    var req MedicalHistory
    json.NewDecoder(r.Body).Decode(&req)
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getMedicalHistory(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    var history []MedicalHistory
    db.Where("patient_id = ?", userID).Find(&history)
    sendJSON(w, history, http.StatusOK)
}

func addSurgery(w http.ResponseWriter, r *http.Request) {
    var req Surgery
    json.NewDecoder(r.Body).Decode(&req)
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getSurgeries(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    var surgeries []Surgery
    db.Where("patient_id = ?", userID).Find(&surgeries)
    sendJSON(w, surgeries, http.StatusOK)
}

func voiceInput(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "text": "Головная боль, температура 37.5", "status": "processing"}, http.StatusOK)
}

func photoInput(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "text": "Парацетамол 500мг 3 раза в день", "status": "processing"}, http.StatusOK)
}

func grantDoctorAccess(w http.ResponseWriter, r *http.Request) {
    var req struct {
        PatientID   uint   `json:"patient_id"`
        DoctorEmail string `json:"doctor_email"`
        AccessType  string `json:"access_type"`
        Hours       int    `json:"hours"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    var doctor User
    db.Where("email = ? AND role = ?", req.DoctorEmail, "doctor").First(&doctor)
    
    access := DoctorAccess{
        PatientID:  req.PatientID,
        DoctorID:   doctor.ID,
        AccessType: req.AccessType,
        ExpiresAt:  time.Now().Add(time.Duration(req.Hours) * time.Hour),
        CreatedAt:  time.Now(),
    }
    db.Create(&access)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getMyPatients(w http.ResponseWriter, r *http.Request) {
    doctorID := r.URL.Query().Get("doctor_id")
    var accesses []DoctorAccess
    db.Where("doctor_id = ?", doctorID).Find(&accesses)
    
    var patients []User
    for _, a := range accesses {
        var patient User
        db.First(&patient, a.PatientID)
        patients = append(patients, patient)
    }
    sendJSON(w, patients, http.StatusOK)
}

func getPatientData(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientID := vars["id"]
    
    var symptoms []Symptom
    var medications []Medication
    var allergies []Allergy
    var history []MedicalHistory
    var surgeries []Surgery
    
    db.Where("patient_id = ?", patientID).Find(&symptoms)
    db.Where("patient_id = ?", patientID).Find(&medications)
    db.Where("patient_id = ?", patientID).Find(&allergies)
    db.Where("patient_id = ?", patientID).Find(&history)
    db.Where("patient_id = ?", patientID).Find(&surgeries)
    
    sendJSON(w, map[string]interface{}{
        "symptoms":    symptoms,
        "medications": medications,
        "allergies":   allergies,
        "history":     history,
        "surgeries":   surgeries,
    }, http.StatusOK)
}

func addExamination(w http.ResponseWriter, r *http.Request) {
    var req Examination
    json.NewDecoder(r.Body).Decode(&req)
    req.ExaminedAt = time.Now()
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func createPrescription(w http.ResponseWriter, r *http.Request) {
    var req Prescription
    json.NewDecoder(r.Body).Decode(&req)
    req.Number = "RX" + time.Now().Format("20060102150405")
    req.IssuedAt = time.Now()
    req.ExpiresAt = time.Now().AddDate(0, 1, 0)
    req.Status = "active"
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true, "number": req.Number}, http.StatusOK)
}

func createCertificate(w http.ResponseWriter, r *http.Request) {
    var req Certificate
    json.NewDecoder(r.Body).Decode(&req)
    req.Number = "SP" + time.Now().Format("20060102150405")
    req.IssuedAt = time.Now()
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true, "number": req.Number}, http.StatusOK)
}

func requestLevel1Analysis(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 1, "result": "Базовый анализ: симптомов не выявлено"}, http.StatusOK)
}

func requestLevel2Analysis(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 2, "result": "Аудит: рисков не обнаружено"}, http.StatusOK)
}

func requestLevel3Analysis(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 3, "result": "Диагностический поиск: требуется дополнительное обследование"}, http.StatusOK)
}

func uploadPrompt(w http.ResponseWriter, r *http.Request) {
    var req AIPrompt
    json.NewDecoder(r.Body).Decode(&req)
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func uploadGuideline(w http.ResponseWriter, r *http.Request) {
    var req ClinicalGuideline
    json.NewDecoder(r.Body).Decode(&req)
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func adminLogin(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    if req.Username == "admin" && req.Password == "admin123" {
        sendJSON(w, map[string]interface{}{"success": true, "token": "admin_token_123"}, http.StatusOK)
    } else {
        sendJSON(w, map[string]interface{}{"success": false, "error": "Invalid credentials"}, http.StatusUnauthorized)
    }
}

func adminDashboard(w http.ResponseWriter, r *http.Request) {
    var totalUsers int64
    db.Model(&User{}).Count(&totalUsers)
    sendJSON(w, map[string]interface{}{
        "total_users":   totalUsers,
        "total_doctors": 0,
        "errors_24h":    0,
        "total_feedback": 0,
        "avg_rating":    4.5,
    }, http.StatusOK)
}

func getLogs(w http.ResponseWriter, r *http.Request) {
    var logs []SystemLog
    db.Order("created_at DESC").Limit(100).Find(&logs)
    sendJSON(w, map[string]interface{}{"logs": logs}, http.StatusOK)
}

func getErrors(w http.ResponseWriter, r *http.Request) {
    var errors []SystemError
    db.Order("created_at DESC").Limit(50).Find(&errors)
    sendJSON(w, map[string]interface{}{"errors": errors}, http.StatusOK)
}

func getFeedback(w http.ResponseWriter, r *http.Request) {
    var feedback []UserFeedback
    db.Order("created_at DESC").Find(&feedback)
    sendJSON(w, map[string]interface{}{"feedback": feedback}, http.StatusOK)
}

func getMetrics(w http.ResponseWriter, r *http.Request) {
    var metrics []PerformanceMetric
    db.Order("created_at DESC").Limit(100).Find(&metrics)
    sendJSON(w, map[string]interface{}{"metrics": metrics}, http.StatusOK)
}

func createAlert(w http.ResponseWriter, r *http.Request) {
    var req Alert
    json.NewDecoder(r.Body).Decode(&req)
    req.CreatedAt = time.Now()
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"status": "pending"}, http.StatusOK)
}

func getAlerts(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    var alerts []Alert
    db.Where("user_id = ?", userID).Find(&alerts)
    sendJSON(w, map[string]interface{}{"alerts": alerts}, http.StatusOK)
}

func submitFeedback(w http.ResponseWriter, r *http.Request) {
    var req UserFeedback
    json.NewDecoder(r.Body).Decode(&req)
    req.CreatedAt = time.Now()
    req.Status = "pending"
    db.Create(&req)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"status": "ok", "time": time.Now().Format(time.RFC3339)}, http.StatusOK)
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        return
    }
    defer conn.Close()
    
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            conn.WriteJSON(map[string]interface{}{
                "type":    "alert",
                "title":   "Системное оповещение",
                "message": "Сервер работает нормально",
            })
        }
    }
}

// HTML страницы
func homePage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>BOT_MAX - Медицинская платформа</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:Arial;background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);min-height:100vh}
.admin-btn{position:fixed;top:20px;right:20px;background:rgba(255,255,255,0.2);color:white;padding:10px 20px;border-radius:30px;text-decoration:none}
.container{max-width:1200px;margin:0 auto;padding:20px}
.header{text-align:center;padding:60px 20px;color:white}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:30px;padding:40px}
.card{background:white;border-radius:20px;padding:30px;text-align:center}
.btn{display:inline-block;padding:12px 30px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border-radius:30px;text-decoration:none}
</style>
</head>
<body>
<a href="/admin" class="admin-btn">🔐 Админ-панель</a>
<div class="container">
<div class="header"><h1>🏥 BOT_MAX</h1><p>Медицинская платформа с ИИ</p></div>
<div class="cards">
<div class="card"><h3>👨‍⚕️ Я пациент</h3><p>Вносите симптомы, получайте рекомендации</p><a href="/patient" class="btn">Войти</a></div>
<div class="card"><h3>👩‍⚕️ Я врач</h3><p>Управляйте пациентами, назначайте лечение</p><a href="/doctor" class="btn">Войти</a></div>
</div>
</div>
</body>
</html>`)
}

func patientPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Кабинет пациента</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:Arial;background:#f0f2f5}
.header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px}
.container{max-width:800px;margin:0 auto;padding:20px}
.card{background:white;border-radius:15px;padding:20px;margin-bottom:20px}
input,textarea,select{width:100%;padding:10px;margin:10px 0;border:1px solid #ddd;border-radius:8px}
button{background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;padding:10px 20px;border-radius:8px;cursor:pointer}
.voice-btn{background:#28a745;margin-right:10px}
.photo-btn{background:#17a2b8}
</style>
</head>
<body>
<div class="header"><h2>🏥 Кабинет пациента</h2><a href="/" style="color:white">Выйти</a></div>
<div class="container">
<div class="card"><h3>📝 Добавить симптом</h3>
<button class="voice-btn" onclick="alert('Голосовой ввод: скажите симптом')">🎤 Голосовой ввод</button>
<button class="photo-btn" onclick="alert('Фото распознавание')">📸 Загрузить фото</button>
<input type="text" id="symptom" placeholder="Симптом">
<input type="range" id="severity" min="1" max="10"><span id="sev">5</span>
<button onclick="addSymptom()">Добавить</button>
</div>
<div class="card"><h3>💊 Мои препараты</h3><div id="meds"></div></div>
</div>
<script>
document.getElementById('severity').oninput=function(){document.getElementById('sev').innerText=this.value}
function addSymptom(){alert('Симптом добавлен')}
</script>
</body>
</html>`)
}

func doctorPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Кабинет врача</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:Arial;background:#f0f2f5}
.header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:20px}
.container{max-width:1200px;margin:0 auto;padding:20px;display:grid;grid-template-columns:300px 1fr;gap:20px}
.patients{background:white;border-radius:15px;padding:20px}
.content{background:white;border-radius:15px;padding:20px}
button{background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;padding:10px;border-radius:8px;cursor:pointer;margin:5px}
</style>
</head>
<body>
<div class="header"><h2>👩‍⚕️ Кабинет врача</h2><a href="/" style="color:white">Выйти</a></div>
<div class="container">
<div class="patients"><h3>Мои пациенты</h3><div id="patients"></div></div>
<div class="content"><h3>Данные пациента</h3><div id="data"></div>
<button onclick="alert('AI анализ')">🧠 AI анализ</button>
<button onclick="alert('Рецепт создан')">💊 Создать рецепт</button>
<button onclick="alert('Справка создана')">📄 Справка</button>
</div>
</div>
</body>
</html>`)
}

func adminPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Админ-панель</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:Arial;background:#1a1a2e;color:white}
.login-container{display:flex;justify-content:center;align-items:center;min-height:100vh;background:linear-gradient(135deg,#667eea,#764ba2)}
.login-card{background:white;padding:40px;border-radius:20px;color:#333}
input{width:100%;padding:10px;margin:10px 0;border:1px solid #ddd;border-radius:8px}
button{width:100%;padding:10px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;border-radius:8px;cursor:pointer}
.dashboard{display:none}
.header{background:#16213e;padding:20px}
.stats{display:grid;grid-template-columns:repeat(4,1fr);gap:20px;padding:20px}
.stat-card{background:#16213e;padding:20px;border-radius:15px}
</style>
</head>
<body>
<div id="login" class="login-container"><div class="login-card"><h2>🔐 Вход в админ-панель</h2><input type="text" id="user" placeholder="Логин"><input type="password" id="pass" placeholder="Пароль"><button onclick="login()">Войти</button></div></div>
<div id="dashboard" class="dashboard"><div class="header"><h2>🏥 Админ-панель</h2><button onclick="logout()" style="background:none;border:1px solid white">Выйти</button></div><div class="stats"><div class="stat-card">Пользователи<br><span id="users">0</span></div></div></div>
<script>
function login(){var u=document.getElementById('user').value,p=document.getElementById('pass').value;if(u=='admin'&&p=='admin123'){document.getElementById('login').style.display='none';document.getElementById('dashboard').style.display='block';fetch('/api/admin/dashboard').then(r=>r.json()).then(d=>document.getElementById('users').innerText=d.total_users)}else alert('Ошибка')}
function logout(){document.getElementById('login').style.display='flex';document.getElementById('dashboard').style.display='none'}
</script>
</body>
</html>`)
}

