package main

import (
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
    
    "github.com/gorilla/mux"
)

type Patient struct {
    ID         string    `json:"id"`
    FullName   string    `json:"full_name"`
    BirthDate  time.Time `json:"birth_date"`
    Email      string    `json:"email"`
    CardNumber string    `json:"card_number"`
    Password   string    `json:"password"`
    Height     int       `json:"height"`
    Weight     float64   `json:"weight"`
    WeightUnit string    `json:"weight_unit"`
}

type WeightHistory struct {
    ID         string    `json:"id"`
    PatientID  string    `json:"patient_id"`
    Height     int       `json:"height"`
    Weight     float64   `json:"weight"`
    WeightUnit string    `json:"weight_unit"`
    RecordedAt time.Time `json:"recorded_at"`
}

type Allergy struct {
    ID       string `json:"id"`
    Allergen string `json:"allergen"`
    Reaction string `json:"reaction"`
}

type Vaccination struct {
    ID   string    `json:"id"`
    Name string    `json:"name"`
    Date time.Time `json:"date"`
}

type Symptom struct {
    ID        string    `json:"id"`
    Text      string    `json:"text"`
    Date      time.Time `json:"date"`
    Photo     string    `json:"photo,omitempty"`
}

type Medication struct {
    ID         string              `json:"id"`
    Name       string              `json:"name"`
    Dosage     string              `json:"dosage"`
    Frequency  string              `json:"frequency"`
    Times      []string            `json:"times"`
    StartDate  time.Time           `json:"start_date"`
    Duration   string              `json:"duration"`
    TakenLogs  map[string]string   `json:"taken_logs"`
    Status     string              `json:"status"`
}

type Diagnosis struct {
    ID          string    `json:"id"`
    PatientID   string    `json:"patient_id"`
    Name        string    `json:"name"`
    Date        time.Time `json:"date"`
    Medications []string  `json:"medications"`
    IsPermanent bool      `json:"is_permanent"`
}

type UploadedFile struct {
    ID        string    `json:"id"`
    PatientID string    `json:"patient_id"`
    Name      string    `json:"name"`
    Path      string    `json:"path"`
    CreatedAt time.Time `json:"created_at"`
}

var (
    patients       = make(map[string]Patient)
    weightHistory  = make(map[string][]WeightHistory)
    allergies      = make(map[string][]Allergy)
    vaccinations   = make(map[string][]Vaccination)
    symptoms       = make(map[string][]Symptom)
    medications    = make(map[string][]Medication)
    diagnoses      = make(map[string][]Diagnosis)
    uploadedFiles  = make(map[string][]UploadedFile)
    mu             sync.RWMutex
)

func init() {
    patients["PAT1"] = Patient{
        ID:         "PAT1",
        FullName:   "Иванов Иван Иванович",
        BirthDate:  time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC),
        Email:      "patient@demo.com",
        CardNumber: "MC001",
        Password:   "123",
        Height:     175,
        Weight:     70,
        WeightUnit: "кг",
    }
    
    weightHistory["PAT1"] = []WeightHistory{
        {ID: "WH1", PatientID: "PAT1", Height: 175, Weight: 70, WeightUnit: "кг", RecordedAt: time.Now()},
    }
    
    allergies["PAT1"] = []Allergy{
        {ID: "AL1", Allergen: "Пенициллин", Reaction: "Крапивница, отек"},
    }
    
    vaccinations["PAT1"] = []Vaccination{
        {ID: "VAC1", Name: "COVID-19", Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
        {ID: "VAC2", Name: "Грипп", Date: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)},
    }
    
    medications["PAT1"] = []Medication{
        {ID: "MED1", Name: "", Dosage: "", Frequency: "", Times: []string{}, StartDate: time.Now(), Duration: "", Status: "active", TakenLogs: make(map[string]string)},
    }
    
    diagnoses["PAT1"] = []Diagnosis{
        {ID: "DIA1", PatientID: "PAT1", Name: "ОРВИ", Date: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), Medications: []string{}, IsPermanent: false},
        {ID: "DIA2", PatientID: "PAT1", Name: "Гипертония", Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Medications: []string{}, IsPermanent: true},
    }
}

func main() {
    os.MkdirAll("uploads", 0755)
    
    r := mux.NewRouter()
    
    r.HandleFunc("/", homePage)
    r.HandleFunc("/dashboard/{id}", dashboardPage)
    r.HandleFunc("/uploads/{file}", serveFile)
    r.HandleFunc("/api/login", loginHandler).Methods("POST")
    r.HandleFunc("/api/patient/{id}", getPatient).Methods("GET")
    r.HandleFunc("/api/patient/{id}", updatePatient).Methods("PUT")
    r.HandleFunc("/api/patient/{id}/weight-history", getWeightHistory).Methods("GET")
    r.HandleFunc("/api/patient/{id}/weight-history", addWeightHistory).Methods("POST")
    r.HandleFunc("/api/allergies/{id}", getAllergies).Methods("GET")
    r.HandleFunc("/api/allergies/{id}", addAllergy).Methods("POST")
    r.HandleFunc("/api/allergies/{id}/{allergyId}", updateAllergy).Methods("PUT")
    r.HandleFunc("/api/allergies/{id}/{allergyId}", deleteAllergy).Methods("DELETE")
    r.HandleFunc("/api/vaccinations/{id}", getVaccinations).Methods("GET")
    r.HandleFunc("/api/vaccinations/{id}", addVaccination).Methods("POST")
    r.HandleFunc("/api/vaccinations/{id}/{vaccId}", updateVaccination).Methods("PUT")
    r.HandleFunc("/api/vaccinations/{id}/{vaccId}", deleteVaccination).Methods("DELETE")
    r.HandleFunc("/api/symptoms/{id}", getSymptoms).Methods("GET")
    r.HandleFunc("/api/symptoms/{id}", addSymptom).Methods("POST")
    r.HandleFunc("/api/symptoms/{id}/{symId}", deleteSymptom).Methods("DELETE")
    r.HandleFunc("/api/medications/{id}", getMedications).Methods("GET")
    r.HandleFunc("/api/medications/{id}", addMedication).Methods("POST")
    r.HandleFunc("/api/medications/{id}/{medId}", updateMedication).Methods("PUT")
    r.HandleFunc("/api/medications/{id}/{medId}", deleteMedication).Methods("DELETE")
    r.HandleFunc("/api/diagnoses/{id}", getDiagnoses).Methods("GET")
    r.HandleFunc("/api/diagnoses/{id}", addDiagnosis).Methods("POST")
    r.HandleFunc("/api/diagnoses/{id}/{diaId}", updateDiagnosis).Methods("PUT")
    r.HandleFunc("/api/diagnoses/{id}/{diaId}", deleteDiagnosis).Methods("DELETE")
    r.HandleFunc("/api/upload", uploadFile).Methods("POST")
    r.HandleFunc("/api/uploads/{id}", getUploadedFiles).Methods("GET")
    r.HandleFunc("/api/uploads/{id}/{fileId}", deleteUploadedFile).Methods("DELETE")
    r.HandleFunc("/api/voice/record", voiceRecordHandler).Methods("POST")
    r.HandleFunc("/api/voice/upload", voiceUploadHandler).Methods("POST")
    r.HandleFunc("/api/print", printHandler).Methods("POST")
    r.HandleFunc("/health", healthCheck)
    
    port := "8082"
    log.Printf("Сервер запущен на http://localhost:%s", port)
    log.Fatal(http.ListenAndServe(":"+port, r))
}

// Исправленная функция serveFile - работает как в тестовом сервере
func serveFile(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    filename := vars["file"]
    filePath := filepath.Join("uploads", filename)
    
    // Проверяем существование файла
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        http.Error(w, "File not found", http.StatusNotFound)
        return
    }
    
    // Определяем Content-Type
    contentType := "application/octet-stream"
    if strings.HasSuffix(filePath, ".pdf") {
        contentType = "application/pdf"
    } else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".jpeg") {
        contentType = "image/jpeg"
    } else if strings.HasSuffix(filePath, ".png") {
        contentType = "image/png"
    }
    
    w.Header().Set("Content-Type", contentType)
    w.Header().Set("Content-Disposition", "inline")
    http.ServeFile(w, r, filePath)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"status": "ok", "time": time.Now().Unix()}, http.StatusOK)
}

func homePage(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprint(w, `<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><title>BOT_MAX</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Segoe UI',Arial,sans-serif;background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);min-height:100vh}
.container{max-width:450px;margin:0 auto;padding:40px}
.card{background:white;border-radius:30px;padding:40px;text-align:center}
h1{color:#667eea}input{width:100%;padding:15px;margin:10px 0;border:2px solid #e0e0e0;border-radius:15px}
button{width:100%;padding:15px;background:linear-gradient(135deg,#667eea,#764ba2);color:white;border:none;border-radius:15px;cursor:pointer}
</style>
</head>
<body>
<div class="container"><div class="card"><h1>🏥 BOT_MAX</h1>
<input type="text" id="email" placeholder="Email" value="patient@demo.com">
<input type="password" id="password" placeholder="Пароль" value="123">
<button onclick="login()">Войти</button></div></div>
<script>
async function login(){
    const res=await fetch('/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({email:document.getElementById('email').value,password:document.getElementById('password').value})});
    const data=await res.json();
    if(data.success) window.location.href='/dashboard/'+data.patient_id;
    else alert('Ошибка');
}
</script>
</body></html>`)
}

func dashboardPage(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Личный кабинет пациента</title>
    <style>
        *{margin:0;padding:0;box-sizing:border-box}
        body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5;padding:20px}
        .header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:15px 20px;border-radius:15px;margin-bottom:20px;display:flex;justify-content:space-between}
        .header a{color:white;text-decoration:none}
        .section{background:white;border-radius:15px;padding:20px;margin-bottom:20px}
        .section-title{font-size:18px;font-weight:bold;color:#667eea;margin-bottom:15px;border-bottom:2px solid #667eea;padding-bottom:10px;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:10px;cursor:pointer}
        .section-content{display:none;margin-top:15px}
        .section-content.show{display:block}
        .info-row{display:flex;flex-wrap:wrap;gap:15px;margin-top:10px}
        .info-item{background:#f8f9fa;padding:10px 15px;border-radius:8px;flex:1}
        .info-label{font-size:11px;color:#666}
        .info-value{font-size:14px;font-weight:bold;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
        .btn-icon{background:none;border:none;cursor:pointer;color:#667eea;font-size:14px;padding:2px 6px;border-radius:5px}
        .item-list{display:grid;gap:10px}
        .item{background:#f8f9fa;padding:12px;border-radius:10px;border-left:3px solid #667eea;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:10px}
        .btn-add{background:#28a745;color:white;border:none;padding:6px 12px;border-radius:15px;cursor:pointer}
        .btn-edit{background:#ffc107;color:#333;border:none;padding:4px 10px;border-radius:15px;cursor:pointer}
        .btn-delete{background:#dc3545;color:white;border:none;padding:4px 10px;border-radius:15px;cursor:pointer}
        .voice-btn{background:#28a745;color:white;border:none;padding:8px 16px;border-radius:20px;cursor:pointer}
        .photo-btn{background:#17a2b8;color:white;border:none;padding:8px 16px;border-radius:20px;cursor:pointer}
        .temp-btn{background:#ffc107;color:#333;border:none;padding:8px 16px;border-radius:20px;cursor:pointer}
        .symptom-area{display:flex;gap:15px;flex-wrap:wrap}
        .symptom-input{flex:2;min-width:300px;height:100px;padding:10px;border:2px solid #e0e0e0;border-radius:10px;font-family:inherit;resize:vertical}
        .file-list{flex:1;min-width:200px;background:#f8f9fa;border-radius:10px;padding:10px;max-height:150px;overflow-y:auto}
        .file-list-title{font-size:12px;font-weight:bold;color:#667eea;margin-bottom:8px}
        .file-item{font-size:11px;padding:4px 8px;margin:2px 0;background:white;border-radius:5px;cursor:pointer;display:flex;justify-content:space-between;align-items:center}
        .delete-file{color:#dc3545;cursor:pointer;margin-left:8px}
        .button-group{display:flex;gap:10px;margin-top:10px;flex-wrap:wrap}
        .modal{display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.5);justify-content:center;align-items:center;z-index:1000}
        .modal-content{background:white;padding:20px;border-radius:15px;width:90%;max-width:500px;position:relative}
        .modal-close{position:absolute;top:10px;right:15px;font-size:24px;cursor:pointer;color:#999}
        .temp-select{display:none;margin-top:5px}
        .voice-options{display:none;margin-top:5px;gap:10px}
        .print-options{display:none;margin-top:5px}
        .arrow{font-size:14px;margin-left:10px}
    </style>
</head>
<body>
<div class="header"><h2>🏥 Личный кабинет пациента</h2><a href="/">Выйти</a></div>
<div id="patientInfo" class="section"></div>

<div class="section">
    <div class="section-title" onclick="toggleSection(this)">📝 Добавить симптом <span class="arrow">▼</span></div>
    <div class="section-content show">
        <div class="symptom-area">
            <textarea id="symptomText" class="symptom-input" placeholder="Опишите симптом..."></textarea>
            <div class="file-list">
                <div class="file-list-title">📎 Загруженные файлы</div>
                <div id="uploadedFilesList"></div>
            </div>
        </div>
        <div class="button-group">
            <button class="temp-btn" onclick="toggleTempSelect()">🌡️ Добавить температуру</button>
            <button class="voice-btn" onclick="toggleVoiceOptions()">🎤 Голосовой ввод</button>
            <button class="photo-btn" onclick="uploadFile()">📸 Загрузить фото</button>
            <button onclick="addSymptom()" style="background:#667eea;color:white;border:none;padding:8px 16px;border-radius:20px;cursor:pointer">💾 Сохранить симптом</button>
            <button class="btn-add" onclick="togglePrintOptions()">🖨️ Распечатать</button>
        </div>
        <div id="tempSelect" class="temp-select"><select id="tempValue"><option>36.0</option><option>36.1</option><option>36.2</option><option>36.3</option><option>36.4</option><option>36.5</option><option selected>36.6</option><option>36.7</option><option>36.8</option><option>36.9</option><option>37.0</option><option>37.1</option><option>37.2</option><option>37.3</option><option>37.4</option><option>37.5</option><option>38.0</option><option>38.5</option><option>39.0</option></select><button onclick="addTemperature()">OK</button></div>
        <div id="voiceOptions" class="voice-options"><button onclick="startLiveVoice()">🎙️ Запись с микрофона</button><button onclick="uploadVoiceFile()">📁 Загрузить аудиофайл</button></div>
        <div id="printOptions" class="print-options"><label><input type="checkbox" id="printSymptoms"> Симптомы</label><label><input type="checkbox" id="printAllergies"> Аллергии</label><label><input type="checkbox" id="printVaccinations"> Вакцинации</label><label><input type="checkbox" id="printMedications"> Препараты</label><label><input type="checkbox" id="printDiagnoses"> Диагнозы</label><button onclick="printSelected()">Печать</button></div>
        <div id="symptomsList" class="item-list" style="margin-top:15px"></div>
    </div>
</div>

<div class="section">
    <div class="section-title" onclick="toggleSection(this)">⚠️ Аллергии <span class="arrow">▼</span><button class="btn-add" onclick="event.stopPropagation();showAddAllergy()">+ Добавить</button></div>
    <div class="section-content show"><div id="allergiesList" class="item-list"></div></div>
</div>

<div class="section">
    <div class="section-title" onclick="toggleSection(this">💉 Вакцинации <span class="arrow">▼</span><button class="btn-add" onclick="event.stopPropagation();showAddVaccination()">+ Добавить</button></div>
    <div class="section-content show"><div id="vaccinationsList" class="item-list"></div></div>
</div>

<div class="section">
    <div class="section-title" onclick="toggleSection(this)">💊 Мои препараты <span class="arrow">▼</span><button class="btn-add" onclick="event.stopPropagation();showAddMedication()">+ Добавить</button></div>
    <div class="section-content show"><div id="medicationsList" class="item-list"></div></div>
</div>

<div class="section">
    <div class="section-title" onclick="toggleSection(this)">📋 Диагнозы <span class="arrow">▼</span><button class="btn-add" onclick="event.stopPropagation();showAddDiagnosis()">+ Добавить</button></div>
    <div class="section-content show"><div id="diagnosesList" class="item-list"></div></div>
</div>

<div id="allergyModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('allergyModal')">&times;</span><h3>Аллергия</h3><input type="text" id="allergenName" placeholder="Аллерген" style="width:100%;margin:10px 0;padding:8px"><input type="text" id="allergenReaction" placeholder="Реакция" style="width:100%;margin:10px 0;padding:8px"><input type="hidden" id="editAllergyId"><button onclick="saveAllergy()">Сохранить</button></div></div>
<div id="vaccinationModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('vaccinationModal')">&times;</span><h3>Вакцинация</h3><input type="text" id="vaccineName" placeholder="Название" style="width:100%;margin:10px 0;padding:8px"><input type="date" id="vaccineDate" style="width:100%;margin:10px 0;padding:8px"><input type="hidden" id="editVaccineId"><button onclick="saveVaccination()">Сохранить</button></div></div>
<div id="medicationModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('medicationModal')">&times;</span><h3>Препарат</h3><input type="text" id="medName" placeholder="Название" style="width:100%;margin:10px 0;padding:8px"><input type="text" id="medDosage" placeholder="Дозировка" style="width:100%;margin:10px 0;padding:8px"><div id="medTimesList"></div><button type="button" onclick="addMedTimeField()">+ Добавить время</button><input type="date" id="medStartDate" style="width:100%;margin:10px 0;padding:8px"><select id="medDuration" style="width:100%;margin:10px 0;padding:8px"><option value="3 дня">3 дня</option><option value="5 дней">5 дней</option><option value="7 дней" selected>7 дней</option><option value="10 дней">10 дней</option><option value="14 дней">14 дней</option><option value="Постоянно">Постоянно</option></select><input type="hidden" id="editMedId"><button onclick="saveMedication()">Сохранить</button></div></div>
<div id="diagnosisModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('diagnosisModal')">&times;</span><h3>Диагноз</h3><input type="text" id="diagnosisName" placeholder="Название" style="width:100%;margin:10px 0;padding:8px"><input type="date" id="diagnosisDate" style="width:100%;margin:10px 0;padding:8px"><input type="hidden" id="editDiagnosisId"><button onclick="saveDiagnosis()">Сохранить</button></div></div>
<div id="weightHistoryModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('weightHistoryModal')">&times;</span><h3>История веса</h3><div id="weightHistoryList"></div></div></div>

<script>
const patientId = "` + patientId + `";
let currentEditAllergyId = null, currentEditVaccId = null, currentEditMedId = null, currentEditDiagnosisId = null;

function toggleSection(el) {
    const content = el.nextElementSibling;
    const arrow = el.querySelector('.arrow');
    content.classList.toggle('show');
    arrow.textContent = content.classList.contains('show') ? '▼' : '▶';
}

async function loadAllData() {
    await loadPatient();
    await loadAllergies();
    await loadVaccinations();
    await loadSymptoms();
    await loadMedications();
    await loadDiagnoses();
    await loadUploadedFiles();
}

function viewFile(path) {
    window.open('/uploads/' + path, '_blank');
}

async function loadUploadedFiles() {
    const res = await fetch('/api/uploads/' + patientId);
    const files = await res.json();
    const container = document.getElementById('uploadedFilesList');
    if (files.length === 0) {
        container.innerHTML = '<div class="file-item">Нет файлов</div>';
    } else {
        container.innerHTML = files.map(f => '<div class="file-item" onclick="viewFile(\'' + f.path + '\')">📄 ' + f.name + '<span class="delete-file" onclick="event.stopPropagation();deleteFile(\'' + f.id + '\')">❌</span></div>').join('');
    }
}

async function deleteFile(fileId) {
    if (confirm('Удалить файл?')) {
        await fetch('/api/uploads/' + patientId + '/' + fileId, { method: 'DELETE' });
        loadUploadedFiles();
    }
}

async function loadPatient() {
    const res = await fetch('/api/patient/' + patientId);
    const p = await res.json();
    const bmi = (p.weight / ((p.height/100)*(p.height/100))).toFixed(1);
    let cat = '';
    if (bmi < 18.5) cat = 'Недостаточный вес';
    else if (bmi < 25) cat = 'Нормальный вес';
    else if (bmi < 30) cat = 'Избыточный вес';
    else cat = 'Ожирение';
    
    document.getElementById('patientInfo').innerHTML = '<div class="section-title">👤 Информация о пациенте</div><div class="info-row">' +
        '<div class="info-item"><div class="info-label">ФИО</div><div class="info-value">' + p.full_name + '<button class="btn-icon" onclick="editField(\'full_name\',\'' + p.full_name + '\')">✏️</button></div></div>' +
        '<div class="info-item"><div class="info-label">Дата рождения</div><div class="info-value">' + new Date(p.birth_date).toLocaleDateString() + '<button class="btn-icon" onclick="editField(\'birth_date\',\'' + new Date(p.birth_date).toISOString().split('T')[0] + '\')">✏️</button></div></div>' +
        '<div class="info-item"><div class="info-label">Рост/Вес</div><div class="info-value">' + p.height + ' см / ' + p.weight + ' ' + p.weight_unit + '<button class="btn-icon" onclick="editWeight()">✏️</button><button class="btn-icon" onclick="showWeightHistory()">📊</button></div></div>' +
        '<div class="info-item"><div class="info-label">ИМТ</div><div class="info-value">' + bmi + ' (' + cat + ')</div></div></div>';
}

function editField(field, value) {
    if (field === 'full_name') {
        const newName = prompt('Введите ФИО:', value);
        if (newName) fetch('/api/patient/' + patientId, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ full_name: newName }) }).then(() => loadPatient());
    } else if (field === 'birth_date') {
        const newDate = prompt('Введите дату рождения (ГГГГ-ММ-ДД):', value);
        if (newDate) fetch('/api/patient/' + patientId, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ birth_date: newDate }) }).then(() => loadPatient());
    }
}

async function editWeight() {
    const h = prompt('Рост (см):', '175');
    const w = prompt('Вес:', '70');
    if (h && w) {
        await fetch('/api/patient/' + patientId + '/weight-history', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ height: parseInt(h), weight: parseFloat(w), weight_unit: 'кг' }) });
        loadPatient();
    }
}

async function showWeightHistory() {
    const res = await fetch('/api/patient/' + patientId + '/weight-history');
    const items = await res.json();
    let html = '<table border=1 style="width:100%"><tr><th>Дата</th><th>Рост</th><th>Вес</th></tr>';
    items.forEach(w => html += '<tr><td>' + new Date(w.recorded_at).toLocaleString() + '</td><td>' + w.height + ' см</td><td>' + w.weight + ' ' + w.weight_unit + '</td></tr>');
    html += '</table>';
    document.getElementById('weightHistoryList').innerHTML = html;
    document.getElementById('weightHistoryModal').style.display = 'flex';
}

async function loadAllergies() {
    const res = await fetch('/api/allergies/' + patientId);
    const items = await res.json();
    const c = document.getElementById('allergiesList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет аллергий</div>';
    else c.innerHTML = items.map(a => '<div class="item">⚠️ ' + a.allergen + ' - ' + a.reaction + '<div><button class="btn-edit" onclick="editAllergy(\'' + a.id + '\',\'' + a.allergen + '\',\'' + a.reaction + '\')">✏️</button><button class="btn-delete" onclick="deleteAllergy(\'' + a.id + '\')">🗑️</button></div></div>').join('');
}

function showAddAllergy() { currentEditAllergyId = null; document.getElementById('allergenName').value = ''; document.getElementById('allergenReaction').value = ''; document.getElementById('allergyModal').style.display = 'flex'; }
function editAllergy(id, allergen, reaction) { currentEditAllergyId = id; document.getElementById('allergenName').value = allergen; document.getElementById('allergenReaction').value = reaction; document.getElementById('allergyModal').style.display = 'flex'; }
async function saveAllergy() {
    const data = { allergen: document.getElementById('allergenName').value, reaction: document.getElementById('allergenReaction').value };
    const url = currentEditAllergyId ? '/api/allergies/' + patientId + '/' + currentEditAllergyId : '/api/allergies/' + patientId;
    const method = currentEditAllergyId ? 'PUT' : 'POST';
    await fetch(url, { method: method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    closeModal('allergyModal');
    loadAllergies();
}
async function deleteAllergy(id) { if (confirm('Удалить аллергию?')) await fetch('/api/allergies/' + patientId + '/' + id, { method: 'DELETE' }); loadAllergies(); }

async function loadVaccinations() {
    const res = await fetch('/api/vaccinations/' + patientId);
    const items = await res.json();
    const c = document.getElementById('vaccinationsList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет вакцинаций</div>';
    else c.innerHTML = items.map(v => '<div class="item">💉 ' + v.name + ' - ' + new Date(v.date).toLocaleDateString() + '<div><button class="btn-edit" onclick="editVaccination(\'' + v.id + '\',\'' + v.name + '\',\'' + v.date + '\')">✏️</button><button class="btn-delete" onclick="deleteVaccination(\'' + v.id + '\')">🗑️</button></div></div>').join('');
}

function showAddVaccination() { currentEditVaccId = null; document.getElementById('vaccineName').value = ''; document.getElementById('vaccineDate').value = ''; document.getElementById('vaccinationModal').style.display = 'flex'; }
function editVaccination(id, name, date) { currentEditVaccId = id; document.getElementById('vaccineName').value = name; document.getElementById('vaccineDate').value = date.split('T')[0]; document.getElementById('vaccinationModal').style.display = 'flex'; }
async function saveVaccination() {
    const data = { name: document.getElementById('vaccineName').value, date: document.getElementById('vaccineDate').value };
    const url = currentEditVaccId ? '/api/vaccinations/' + patientId + '/' + currentEditVaccId : '/api/vaccinations/' + patientId;
    const method = currentEditVaccId ? 'PUT' : 'POST';
    await fetch(url, { method: method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    closeModal('vaccinationModal');
    loadVaccinations();
}
async function deleteVaccination(id) { if (confirm('Удалить вакцинацию?')) await fetch('/api/vaccinations/' + patientId + '/' + id, { method: 'DELETE' }); loadVaccinations(); }

async function loadSymptoms() {
    const res = await fetch('/api/symptoms/' + patientId);
    const items = await res.json();
    const c = document.getElementById('symptomsList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет симптомов</div>';
    else c.innerHTML = items.map(s => '<div class="item">📝 ' + s.text + '<button class="btn-delete" onclick="deleteSymptom(\'' + s.id + '\')">🗑️</button></div>').join('');
}

async function deleteSymptom(id) {
    if (confirm('Удалить симптом?')) await fetch('/api/symptoms/' + patientId + '/' + id, { method: 'DELETE' });
    loadSymptoms();
}

async function loadMedications() {
    const res = await fetch('/api/medications/' + patientId);
    const items = await res.json();
    const c = document.getElementById('medicationsList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет препаратов</div>';
    else c.innerHTML = items.map(m => '<div class="item"><strong>💊 ' + (m.name || '—') + ' ' + (m.dosage || '—') + '</strong><br><small>Начало: ' + new Date(m.start_date).toLocaleDateString() + '</small><div><button class="btn-edit" onclick="editMedication(\'' + m.id + '\',\'' + (m.name || '') + '\',\'' + (m.dosage || '') + '\')">✏️</button><button class="btn-delete" onclick="deleteMedication(\'' + m.id + '\')">🗑️</button></div></div>').join('');
}

function showAddMedication() { currentEditMedId = null; document.getElementById('medTimesList').innerHTML = ''; document.getElementById('medName').value = ''; document.getElementById('medDosage').value = ''; document.getElementById('medStartDate').value = ''; document.getElementById('medicationModal').style.display = 'flex'; }
function editMedication(id, name, dosage) { currentEditMedId = id; document.getElementById('medName').value = name; document.getElementById('medDosage').value = dosage; document.getElementById('medicationModal').style.display = 'flex'; }
async function saveMedication() {
    const times = Array.from(document.querySelectorAll('.med-time')).map(t => t.value);
    const data = { name: document.getElementById('medName').value, dosage: document.getElementById('medDosage').value, times: times, start_date: document.getElementById('medStartDate').value, duration: document.getElementById('medDuration').value, frequency: times.length + ' раз в день', status: 'active' };
    const url = currentEditMedId ? '/api/medications/' + patientId + '/' + currentEditMedId : '/api/medications/' + patientId;
    const method = currentEditMedId ? 'PUT' : 'POST';
    await fetch(url, { method: method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    closeModal('medicationModal');
    loadMedications();
}
function addMedTimeField() { const c = document.getElementById('medTimesList'); const d = document.createElement('div'); d.innerHTML = '<input type="time" class="med-time" style="margin:5px;padding:5px"> <button type="button" onclick="this.parentElement.remove()">❌</button>'; c.appendChild(d); }
async function deleteMedication(id) { if (confirm('Удалить препарат?')) await fetch('/api/medications/' + patientId + '/' + id, { method: 'DELETE' }); loadMedications(); }

async function loadDiagnoses() {
    const res = await fetch('/api/diagnoses/' + patientId);
    const items = await res.json();
    const c = document.getElementById('diagnosesList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет диагнозов</div>';
    else c.innerHTML = items.map(d => '<div class="item">📋 ' + d.name + ' - ' + new Date(d.date).toLocaleDateString() + '<div><button class="btn-edit" onclick="editDiagnosis(\'' + d.id + '\',\'' + d.name + '\',\'' + d.date + '\')">✏️</button><button class="btn-delete" onclick="deleteDiagnosis(\'' + d.id + '\')">🗑️</button></div></div>').join('');
}

function showAddDiagnosis() { currentEditDiagnosisId = null; document.getElementById('diagnosisName').value = ''; document.getElementById('diagnosisDate').value = ''; document.getElementById('diagnosisModal').style.display = 'flex'; }
function editDiagnosis(id, name, date) { currentEditDiagnosisId = id; document.getElementById('diagnosisName').value = name; document.getElementById('diagnosisDate').value = date.split('T')[0]; document.getElementById('diagnosisModal').style.display = 'flex'; }
async function saveDiagnosis() {
    const data = { name: document.getElementById('diagnosisName').value, date: document.getElementById('diagnosisDate').value };
    const url = currentEditDiagnosisId ? '/api/diagnoses/' + patientId + '/' + currentEditDiagnosisId : '/api/diagnoses/' + patientId;
    const method = currentEditDiagnosisId ? 'PUT' : 'POST';
    await fetch(url, { method: method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    closeModal('diagnosisModal');
    loadDiagnoses();
}
async function deleteDiagnosis(id) { if (confirm('Удалить диагноз?')) await fetch('/api/diagnoses/' + patientId + '/' + id, { method: 'DELETE' }); loadDiagnoses(); }

function toggleTempSelect() { const sel = document.getElementById('tempSelect'); sel.style.display = sel.style.display === 'none' ? 'block' : 'none'; }
function addTemperature() {
    const temp = document.getElementById('tempValue').value;
    const now = new Date();
    const time = now.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
    const date = now.toLocaleDateString('ru-RU');
    document.getElementById('symptomText').value += '[' + date + ' ' + time + '] Температура: ' + temp + '°C\n';
    document.getElementById('tempSelect').style.display = 'none';
}
function toggleVoiceOptions() { const opt = document.getElementById('voiceOptions'); opt.style.display = opt.style.display === 'none' ? 'flex' : 'none'; }
function startLiveVoice() {
    if ('webkitSpeechRecognition' in window) {
        const r = new webkitSpeechRecognition();
        r.lang = 'ru-RU';
        r.onresult = e => { document.getElementById('symptomText').value += e.results[0][0].transcript + '\n'; };
        r.start();
        document.getElementById('voiceOptions').style.display = 'none';
    } else alert('Голосовой ввод не поддерживается');
}
function uploadVoiceFile() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'audio/*';
    input.onchange = async e => {
        const fd = new FormData();
        fd.append('audio', e.target.files[0]);
        const res = await fetch('/api/voice/upload', { method: 'POST', body: fd });
        const data = await res.json();
        if (data.text) {
            document.getElementById('symptomText').value += data.text + '\n';
            alert('Аудио распознано!');
        }
    };
    input.click();
    document.getElementById('voiceOptions').style.display = 'none';
}
function uploadFile() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*,application/pdf';
    input.onchange = async e => {
        const fd = new FormData();
        fd.append('file', e.target.files[0]);
        fd.append('patientId', patientId);
        const res = await fetch('/api/upload', { method: 'POST', body: fd });
        const data = await res.json();
        if (data.success) { loadUploadedFiles(); alert('Файл загружен!'); }
    };
    input.click();
}
async function addSymptom() {
    let txt = document.getElementById('symptomText').value;
    if (!txt) { alert('Введите симптом'); return; }
    const now = new Date();
    const dateStr = now.toLocaleDateString('ru-RU') + ', ' + now.toLocaleTimeString('ru-RU');
    txt = dateStr + ' - ' + txt;
    await fetch('/api/symptoms/' + patientId, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text: txt }) });
    document.getElementById('symptomText').value = '';
    loadSymptoms();
}
function togglePrintOptions() { const opt = document.getElementById('printOptions'); opt.style.display = opt.style.display === 'none' ? 'block' : 'none'; }
function printSelected() {
    let content = '';
    if (document.getElementById('printSymptoms')?.checked) document.querySelectorAll('#symptomsList .item').forEach(s => content += s.innerText + '\n\n');
    if (document.getElementById('printAllergies')?.checked) document.querySelectorAll('#allergiesList .item').forEach(a => content += a.innerText + '\n\n');
    if (document.getElementById('printVaccinations')?.checked) document.querySelectorAll('#vaccinationsList .item').forEach(v => content += v.innerText + '\n\n');
    if (document.getElementById('printMedications')?.checked) document.querySelectorAll('#medicationsList .item').forEach(m => content += m.innerText + '\n\n');
    if (document.getElementById('printDiagnoses')?.checked) document.querySelectorAll('#diagnosesList .item').forEach(d => content += d.innerText + '\n\n');
    const win = window.open('', '', 'width=800,height=600');
    win.document.write('<html><head><title>Печать</title></head><body><pre>' + content + '</pre></body></html>');
    win.document.close();
    win.print();
}
function closeModal(id) { document.getElementById(id).style.display = 'none'; }

loadAllData();
</script>
</body>
</html>`)
}

func updatePatient(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]
    var req struct {
        FullName  string `json:"full_name"`
        BirthDate string `json:"birth_date"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    mu.Lock()
    if p, exists := patients[id]; exists {
        if req.FullName != "" {
            p.FullName = req.FullName
        }
        if req.BirthDate != "" {
            birthDate, _ := time.Parse("2006-01-02", req.BirthDate)
            p.BirthDate = birthDate
        }
        patients[id] = p
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
    var req struct{ Email, Password string }
    json.NewDecoder(r.Body).Decode(&req)
    for _, p := range patients {
        if p.Email == req.Email && p.Password == req.Password {
            sendJSON(w, map[string]interface{}{"success": true, "patient_id": p.ID}, http.StatusOK)
            return
        }
    }
    sendJSON(w, map[string]interface{}{"success": false}, http.StatusUnauthorized)
}

func getPatient(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    id := vars["id"]
    mu.RLock()
    p, exists := patients[id]
    mu.RUnlock()
    if exists {
        sendJSON(w, p, http.StatusOK)
        return
    }
    sendJSON(w, map[string]interface{}{"error": "Not found"}, http.StatusNotFound)
}

func getWeightHistory(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := weightHistory[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func addWeightHistory(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    var req struct{ Height int; Weight float64; WeightUnit string }
    json.NewDecoder(r.Body).Decode(&req)
    
    mu.Lock()
    if p, exists := patients[vars["id"]]; exists {
        p.Height = req.Height
        p.Weight = req.Weight
        p.WeightUnit = req.WeightUnit
        patients[vars["id"]] = p
    }
    newHistory := WeightHistory{
        ID:         fmt.Sprintf("WH%d", time.Now().UnixNano()),
        PatientID:  vars["id"],
        Height:     req.Height,
        Weight:     req.Weight,
        WeightUnit: req.WeightUnit,
        RecordedAt: time.Now(),
    }
    weightHistory[vars["id"]] = append(weightHistory[vars["id"]], newHistory)
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getAllergies(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := allergies[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func addAllergy(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    var req Allergy
    json.NewDecoder(r.Body).Decode(&req)
    newAllergy := Allergy{ID: fmt.Sprintf("AL%d", time.Now().UnixNano()), Allergen: req.Allergen, Reaction: req.Reaction}
    mu.Lock()
    allergies[vars["id"]] = append(allergies[vars["id"]], newAllergy)
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func updateAllergy(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    allergyId := vars["allergyId"]
    var req Allergy
    json.NewDecoder(r.Body).Decode(&req)
    mu.Lock()
    items := allergies[patientId]
    for i, a := range items {
        if a.ID == allergyId {
            items[i].Allergen = req.Allergen
            items[i].Reaction = req.Reaction
            allergies[patientId] = items
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func deleteAllergy(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    allergyId := vars["allergyId"]
    mu.Lock()
    items := allergies[patientId]
    for i, a := range items {
        if a.ID == allergyId {
            allergies[patientId] = append(items[:i], items[i+1:]...)
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getVaccinations(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := vaccinations[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func addVaccination(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    var req struct{ Name, Date string }
    json.NewDecoder(r.Body).Decode(&req)
    date, _ := time.Parse("2006-01-02", req.Date)
    newVacc := Vaccination{ID: fmt.Sprintf("VAC%d", time.Now().UnixNano()), Name: req.Name, Date: date}
    mu.Lock()
    vaccinations[vars["id"]] = append(vaccinations[vars["id"]], newVacc)
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func updateVaccination(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    vaccId := vars["vaccId"]
    var req struct{ Name, Date string }
    json.NewDecoder(r.Body).Decode(&req)
    date, _ := time.Parse("2006-01-02", req.Date)
    mu.Lock()
    items := vaccinations[patientId]
    for i, v := range items {
        if v.ID == vaccId {
            items[i].Name = req.Name
            items[i].Date = date
            vaccinations[patientId] = items
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func deleteVaccination(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    vaccId := vars["vaccId"]
    mu.Lock()
    items := vaccinations[patientId]
    for i, v := range items {
        if v.ID == vaccId {
            vaccinations[patientId] = append(items[:i], items[i+1:]...)
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getSymptoms(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := symptoms[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func addSymptom(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    var req struct{ Text, Photo string }
    json.NewDecoder(r.Body).Decode(&req)
    newSym := Symptom{ID: fmt.Sprintf("SYM%d", time.Now().UnixNano()), Text: req.Text, Date: time.Now(), Photo: req.Photo}
    mu.Lock()
    symptoms[vars["id"]] = append(symptoms[vars["id"]], newSym)
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func deleteSymptom(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    symId := vars["symId"]
    mu.Lock()
    items := symptoms[patientId]
    for i, s := range items {
        if s.ID == symId {
            symptoms[patientId] = append(items[:i], items[i+1:]...)
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getMedications(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := medications[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func addMedication(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    var req struct {
        Name, Dosage, Frequency, StartDate, Duration string
        Times []string
        Status string
    }
    json.NewDecoder(r.Body).Decode(&req)
    startDate, _ := time.Parse("2006-01-02", req.StartDate)
    newMed := Medication{
        ID:         fmt.Sprintf("MED%d", time.Now().UnixNano()),
        Name:       req.Name,
        Dosage:     req.Dosage,
        Frequency:  req.Frequency,
        Times:      req.Times,
        StartDate:  startDate,
        Duration:   req.Duration,
        Status:     "active",
        TakenLogs:  make(map[string]string),
    }
    mu.Lock()
    medications[vars["id"]] = append(medications[vars["id"]], newMed)
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func updateMedication(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    medId := vars["medId"]
    var req struct {
        Name, Dosage, StartDate string
    }
    json.NewDecoder(r.Body).Decode(&req)
    startDate, _ := time.Parse("2006-01-02", req.StartDate)
    mu.Lock()
    items := medications[patientId]
    for i, m := range items {
        if m.ID == medId {
            items[i].Name = req.Name
            items[i].Dosage = req.Dosage
            items[i].StartDate = startDate
            medications[patientId] = items
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func deleteMedication(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    medId := vars["medId"]
    mu.Lock()
    items := medications[patientId]
    for i, m := range items {
        if m.ID == medId {
            medications[patientId] = append(items[:i], items[i+1:]...)
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getDiagnoses(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := diagnoses[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func addDiagnosis(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    var req Diagnosis
    json.NewDecoder(r.Body).Decode(&req)
    req.ID = fmt.Sprintf("DIA%d", time.Now().UnixNano())
    req.PatientID = vars["id"]
    mu.Lock()
    diagnoses[req.PatientID] = append(diagnoses[req.PatientID], req)
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func updateDiagnosis(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    diaId := vars["diaId"]
    var req Diagnosis
    json.NewDecoder(r.Body).Decode(&req)
    mu.Lock()
    items := diagnoses[patientId]
    for i, d := range items {
        if d.ID == diaId {
            items[i].Name = req.Name
            items[i].Date = req.Date
            diagnoses[patientId] = items
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func deleteDiagnosis(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    diaId := vars["diaId"]
    mu.Lock()
    items := diagnoses[patientId]
    for i, d := range items {
        if d.ID == diaId {
            diagnoses[patientId] = append(items[:i], items[i+1:]...)
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(32 << 20)
    file, handler, err := r.FormFile("file")
    if err != nil {
        sendJSON(w, map[string]interface{}{"success": false, "error": err.Error()}, http.StatusBadRequest)
        return
    }
    defer file.Close()
    
    patientId := r.FormValue("patientId")
    filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), handler.Filename)
    dst, err := os.Create(filepath.Join("uploads", filename))
    if err != nil {
        sendJSON(w, map[string]interface{}{"success": false, "error": err.Error()}, http.StatusInternalServerError)
        return
    }
    defer dst.Close()
    io.Copy(dst, file)
    
    newFile := UploadedFile{
        ID:        fmt.Sprintf("UF%d", time.Now().UnixNano()),
        PatientID: patientId,
        Name:      handler.Filename,
        Path:      filename,
        CreatedAt: time.Now(),
    }
    mu.Lock()
    uploadedFiles[patientId] = append(uploadedFiles[patientId], newFile)
    mu.Unlock()
    
    sendJSON(w, map[string]interface{}{"success": true, "filename": filename}, http.StatusOK)
}

func getUploadedFiles(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := uploadedFiles[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func deleteUploadedFile(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    fileId := vars["fileId"]
    mu.Lock()
    items := uploadedFiles[patientId]
    for i, f := range items {
        if f.ID == fileId {
            os.Remove(filepath.Join("uploads", f.Path))
            uploadedFiles[patientId] = append(items[:i], items[i+1:]...)
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func voiceRecordHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func voiceUploadHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(32 << 20)
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func printHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func sendJSON(w http.ResponseWriter, data interface{}, status int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}
