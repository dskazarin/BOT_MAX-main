#!/bin/bash

# Добавляем импорты в main.go
sed -i '/^import/a \    "encoding/json"\n    "github.com/gorilla/mux"\n    "golang.org/x/crypto/bcrypt"' cmd/server/main.go

# Регистрируем обработчики
cat >> cmd/server/main.go << 'ROUTESEOF'

// Регистрация API маршрутов
func registerRoutes(r *mux.Router) {
    // Публичные маршруты
    r.HandleFunc("/api/register", registerHandler).Methods("POST")
    r.HandleFunc("/api/login", loginHandler).Methods("POST")
    r.HandleFunc("/api/logout", logoutHandler).Methods("POST")
    
    // Пациент
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
    
    // Врач
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
    
    // Админ
    r.HandleFunc("/api/admin/login", adminLogin).Methods("POST")
    r.HandleFunc("/api/admin/dashboard", adminDashboard).Methods("GET")
    r.HandleFunc("/api/admin/logs", getLogs).Methods("GET")
    r.HandleFunc("/api/admin/errors", getErrors).Methods("GET")
    r.HandleFunc("/api/admin/feedback", getFeedback).Methods("GET")
    r.HandleFunc("/api/admin/metrics", getMetrics).Methods("GET")
    
    // Оповещения
    r.HandleFunc("/api/alerts/create", createAlert).Methods("POST")
    r.HandleFunc("/api/alerts", getAlerts).Methods("GET")
    
    // Обратная связь
    r.HandleFunc("/api/feedback", submitFeedback).Methods("POST")
    
    // Health check
    r.HandleFunc("/health", healthCheck).Methods("GET")
}

func createAlert(w http.ResponseWriter, r *http.Request) {
    var req struct {
        UserID  uint   `json:"user_id"`
        Type    string `json:"type"`
        Message string `json:"message"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    alert := Alert{
        UserID:    req.UserID,
        Type:      req.Type,
        Message:   req.Message,
        IsRead:    false,
        CreatedAt: time.Now(),
    }
    db.Create(&alert)
    sendJSON(w, map[string]interface{}{"status": "pending"}, http.StatusOK)
}

func getAlerts(w http.ResponseWriter, r *http.Request) {
    userID := r.URL.Query().Get("user_id")
    var alerts []Alert
    db.Where("user_id = ?", userID).Order("created_at DESC").Find(&alerts)
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
    sendJSON(w, map[string]interface{}{
        "status": "ok",
        "time":   time.Now().Format(time.RFC3339),
    }, http.StatusOK)
}

ROUTESEOF

# Обновляем main.go для использования маршрутов
sed -i '/r.HandleFunc("\/", homePage)/a \    registerRoutes(r)' cmd/server/main.go

