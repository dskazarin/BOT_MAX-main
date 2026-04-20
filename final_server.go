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

// Исправленные AI обработчики
func analysisLevel1(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 1, "result": "level1 completed"}, http.StatusOK)
}

func analysisLevel2(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 2, "result": "level2 completed"}, http.StatusOK)
}

func analysisLevel3(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true, "level": 3, "result": "level3 completed"}, http.StatusOK)
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

// HTML страницы (сокращенные для читаемости)
func homePage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>BOT_MAX</title>
<style>*{margin:0;padding:0;box-sizing:border-box}body{font-family:Arial;background:linear-gradient(135deg,#667eea,#764ba2);min-height:100vh}.admin-btn{position:fixed;top:20px;right:20px;background:rgba(255,255,255,0.2);color:white;padding:10px20px;border-radius:30px;text-decoration:none}.container{max-width:1200px;margin:0 auto;padding:40px}.header{text-align:center;color:white}.cards{display:grid;grid-template-columns:1fr1fr;gap:30px;margin-top:40px}.card{background:white;border-radius:20px;padding:40px;text-align:center}.btn{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:10px30px;border-radius:30px;text-decoration:none}</style>
</head>
<body><a href="/admin" class="admin-btn">🔐 Админ-панель</a>
<div class="container"><div class="header"><h1>🏥 BOT_MAX</h1><p>Медицинская платформа с ИИ</p></div>
<div class="cards"><div class="card"><h2>👨‍⚕️ Пациент</h2><a href="/patient" class="btn">Войти</a></div>
<div class="card"><h2>👩‍⚕️ Врач</h2><a href="/doctor" class="btn">Войти</a></div></div></div>
</body></html>`)
}

func patientPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Пациент</title></head><body><h1>🏥 Кабинет пациента</h1><a href="/">На главную</a></body></html>`)
}

func doctorPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Врач</title></head><body><h1>👩‍⚕️ Кабинет врача</h1><a href="/">На главную</a></body></html>`)
}

func adminPage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html><html><head><meta charset="UTF-8"><title>Админ</title></head><body><h1>🔐 Админ-панель</h1><p>Логин: admin, Пароль: admin123</p><a href="/">На главную</a></body></html>`)
}
