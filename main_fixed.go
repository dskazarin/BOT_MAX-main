package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strings"
    "time"
)

// SQL Injection Protection
func validateInput(input string) bool {
    if input == "" {
        return true
    }
    dangerous := []string{"'", "\"", ";", "--", "DROP", "DELETE", "INSERT", "UPDATE", "UNION", "OR", "AND", "SELECT"}
    upperInput := strings.ToUpper(input)
    for _, d := range dangerous {
        if strings.Contains(upperInput, d) {
            log.Printf("⚠️ SQL Injection blocked: %s", input)
            return false
        }
    }
    return true
}

// XSS Protection (Unicode escaping)
func escapeOutput(input string) string {
    if input == "" {
        return ""
    }
    output := strings.ReplaceAll(input, "&", "&amp;")
    output = strings.ReplaceAll(output, "<", "&lt;")
    output = strings.ReplaceAll(output, ">", "&gt;")
    output = strings.ReplaceAll(output, "\"", "&quot;")
    output = strings.ReplaceAll(output, "'", "&#39;")
    output = strings.ReplaceAll(output, "`", "&#96;")
    output = strings.ReplaceAll(output, "(", "&#40;")
    output = strings.ReplaceAll(output, ")", "&#41;")
    output = strings.ReplaceAll(output, "=", "&#61;")
    return output
}

// Security Headers
func securityHeaders(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Content-Security-Policy", "default-src 'self'")
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        next.ServeHTTP(w, r)
    }
}

var patients = []map[string]interface{}{
    {"id": 1, "name": "John Doe", "age": 45, "diagnosis": "Hypertension", "phone": "+1-555-0101"},
    {"id": 2, "name": "Jane Smith", "age": 32, "diagnosis": "Migraine", "phone": "+1-555-0102"},
    {"id": 3, "name": "Bob Johnson", "age": 58, "diagnosis": "Diabetes", "phone": "+1-555-0103"},
    {"id": 4, "name": "Alice Brown", "age": 28, "diagnosis": "Allergy", "phone": "+1-555-0104"},
    {"id": 5, "name": "Charlie Wilson", "age": 65, "diagnosis": "Arthritis", "phone": "+1-555-0105"},
}

func main() {
    http.HandleFunc("/health", securityHeaders(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "time": time.Now().Unix(), "secure": true})
    }))

    http.HandleFunc("/api/stats", securityHeaders(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]interface{}{
            "patients": len(patients), "status": "SECURE",
            "security": "SQLi Protected, XSS Protected, Headers Active", "timestamp": time.Now().Unix(),
        })
    }))

    http.HandleFunc("/api/patients", securityHeaders(func(w http.ResponseWriter, r *http.Request) {
        patientID := r.URL.Query().Get("id")
        if patientID != "" && !validateInput(patientID) {
            http.Error(w, "Invalid input parameters", http.StatusBadRequest)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        
        escapedPatients := make([]map[string]interface{}, len(patients))
        for i, p := range patients {
            escapedPatients[i] = make(map[string]interface{})
            for k, v := range p {
                if str, ok := v.(string); ok {
                    escapedPatients[i][k] = escapeOutput(str)
                } else {
                    escapedPatients[i][k] = v
                }
            }
        }
        json.NewEncoder(w).Encode(escapedPatients)
    }))

    http.HandleFunc("/", securityHeaders(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html")
        fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>BOT_MAX - Secure Medical System</title>
<style>
body{font-family:Arial;text-align:center;margin:50px;background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);}
.container{background:white;padding:40px;border-radius:20px;max-width:600px;margin:auto;}
h1{color:#667eea;}.secure{color:#4CAF50;font-weight:bold;}
button{background:#667eea;color:white;border:none;padding:10px 20px;margin:10px;border-radius:5px;cursor:pointer;}
</style>
</head>
<body>
<div class="container">
<h1>🏥 BOT_MAX Secure Medical System</h1>
<p class="secure">✅ ALL SECURITY FEATURES ACTIVE</p>
<p>🔒 SQL Injection Protection<br>🔒 XSS Attack Prevention<br>🔒 Security Headers Active</p>
<button onclick="location.href='/api/stats'">📊 Stats</button>
<button onclick="location.href='/api/patients'">📋 Patients</button>
<button onclick="location.href='/health'">❤️ Health</button>
</div>
</body>
</html>`)
    }))

    log.Printf("🚀 BOT_MAX Secure Server starting on port 8082")
    log.Fatal(http.ListenAndServe(":8082", nil))
}
