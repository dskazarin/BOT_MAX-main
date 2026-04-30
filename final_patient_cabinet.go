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
    EndDate    *time.Time          `json:"end_date,omitempty"`
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

type LabResult struct {
    ID       string    `json:"id"`
    Name     string    `json:"name"`
    Result   string    `json:"result"`
    Date     time.Time `json:"date"`
    FilePath string    `json:"file_path,omitempty"`
}

type MedicalRecord struct {
    ID          string    `json:"id"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Date        time.Time `json:"date"`
    DoctorSpec  string    `json:"doctor_spec,omitempty"`
    FilePath    string    `json:"file_path,omitempty"`
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
    labResults     = make(map[string][]LabResult)
    medicalRecords = make(map[string][]MedicalRecord)
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
    
    symptoms["PAT1"] = []Symptom{
        {ID: "SYM1", Text: "[30.04.2026 15:45] Температура: 37.5°C, боль в горле, заложенность носа", Date: time.Now()},
    }
    
    medications["PAT1"] = []Medication{
        {ID: "MED1", Name: "", Dosage: "", Frequency: "", Times: []string{}, StartDate: time.Now(), Duration: "", Status: "active", TakenLogs: make(map[string]string)},
    }
    
    diagnoses["PAT1"] = []Diagnosis{
        {ID: "DIA1", PatientID: "PAT1", Name: "ОРВИ", Date: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), Medications: []string{"Парацетамол"}, IsPermanent: false},
        {ID: "DIA2", PatientID: "PAT1", Name: "Гипертония", Date: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC), Medications: []string{"Эналаприл"}, IsPermanent: true},
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
    r.HandleFunc("/api/patient/{id}/weight-history", getWeightHistory).Methods("GET")
    r.HandleFunc("/api/patient/{id}/weight-history", addWeightHistory).Methods("POST")
    r.HandleFunc("/api/allergies/{id}", getAllergies).Methods("GET")
    r.HandleFunc("/api/allergies/{id}", addAllergy).Methods("POST")
    r.HandleFunc("/api/allergies/{id}/{allergyId}", deleteAllergy).Methods("DELETE")
    r.HandleFunc("/api/vaccinations/{id}", getVaccinations).Methods("GET")
    r.HandleFunc("/api/vaccinations/{id}", addVaccination).Methods("POST")
    r.HandleFunc("/api/vaccinations/{id}/{vaccId}", deleteVaccination).Methods("DELETE")
    r.HandleFunc("/api/symptoms/{id}", getSymptoms).Methods("GET")
    r.HandleFunc("/api/symptoms/{id}", addSymptom).Methods("POST")
    r.HandleFunc("/api/symptoms/{id}/{symId}", deleteSymptom).Methods("DELETE")
    r.HandleFunc("/api/medications/{id}", getMedications).Methods("GET")
    r.HandleFunc("/api/medications/{id}", addMedication).Methods("POST")
    r.HandleFunc("/api/medications/{id}/{medId}", updateMedication).Methods("PUT")
    r.HandleFunc("/api/medications/{id}/{medId}", deleteMedication).Methods("DELETE")
    r.HandleFunc("/api/medications/{id}/{medId}/taken", markTaken).Methods("POST")
    r.HandleFunc("/api/medications/{id}/{medId}/history", getMedicationHistory).Methods("GET")
    r.HandleFunc("/api/diagnoses/{id}", getDiagnoses).Methods("GET")
    r.HandleFunc("/api/diagnoses/{id}/{diaId}/medications", updateDiagnosisMedications).Methods("POST")
    r.HandleFunc("/api/labresults/{id}", getLabResults).Methods("GET")
    r.HandleFunc("/api/records/{id}", getMedicalRecords).Methods("GET")
    r.HandleFunc("/api/upload", uploadFile).Methods("POST")
    r.HandleFunc("/api/uploads/{id}", getUploadedFiles).Methods("GET")
    r.HandleFunc("/api/uploads/{id}/{fileId}", deleteUploadedFile).Methods("DELETE")
    r.HandleFunc("/api/voice/record", voiceRecordHandler).Methods("POST")
    r.HandleFunc("/api/voice/upload", voiceUploadHandler).Methods("POST")
    r.HandleFunc("/api/print", printHandler).Methods("POST")
    
    port := "8082"
    log.Printf("Сервер запущен на http://localhost:%s", port)
    log.Fatal(http.ListenAndServe(":"+port, r))
}

func serveFile(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    file := vars["file"]
    if strings.HasSuffix(file, ".pdf") {
        w.Header().Set("Content-Type", "application/pdf")
        w.Header().Set("Content-Disposition", "inline; filename="+file)
    } else {
        w.Header().Set("Content-Type", "image/jpeg")
    }
    http.ServeFile(w, r, filepath.Join("uploads", file))
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
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>Личный кабинет</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Segoe UI',Arial,sans-serif;background:#f0f2f5;padding:20px}
.header{background:linear-gradient(135deg,#667eea,#764ba2);color:white;padding:15px 20px;border-radius:15px;margin-bottom:20px;display:flex;justify-content:space-between;align-items:center}
.header a{color:white;text-decoration:none}
.section{background:white;border-radius:15px;padding:20px;margin-bottom:20px;box-shadow:0 2px 10px rgba(0,0,0,0.05)}
.section-title{font-size:18px;font-weight:bold;color:#667eea;margin-bottom:15px;border-bottom:2px solid #667eea;padding-bottom:10px;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:10px}
.info-row{display:flex;flex-wrap:wrap;gap:15px;margin-top:10px}
.info-item{background:#f8f9fa;padding:10px 15px;border-radius:8px;flex:1;min-width:150px}
.info-label{font-size:11px;color:#666}
.info-value{font-size:14px;font-weight:bold}
.item-list{display:grid;gap:10px}
.item{background:#f8f9fa;padding:12px;border-radius:10px;border-left:3px solid #667eea;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:10px}
.item-med{background:#f8f9fa;padding:15px;border-radius:10px;border-left:3px solid #28a745;margin-bottom:10px}
.med-times{display:flex;gap:10px;margin:10px 0;flex-wrap:wrap;align-items:center}
.time-badge{background:#e9ecef;padding:6px 12px;border-radius:20px;font-size:12px;cursor:pointer;transition:all 0.2s}
.time-badge.taken{background:#28a745;color:white}
.time-badge.missed{background:#dc3545;color:white}
.btn-add{background:#28a745;color:white;border:none;padding:6px 12px;border-radius:15px;cursor:pointer}
.btn-edit{background:#ffc107;color:#333;border:none;padding:4px 10px;border-radius:15px;cursor:pointer;margin-left:5px}
.btn-delete{background:#dc3545;color:white;border:none;padding:4px 10px;border-radius:15px;cursor:pointer;margin-left:5px}
.btn-icon{background:none;border:none;cursor:pointer;color:#667eea;font-size:16px;margin-left:5px}
.voice-btn{background:#28a745;color:white;border:none;padding:8px 16px;border-radius:20px;cursor:pointer;margin-right:10px}
.photo-btn{background:#17a2b8;color:white;border:none;padding:8px 16px;border-radius:20px;cursor:pointer;margin-right:10px}
.temp-btn{background:#ffc107;color:#333;border:none;padding:8px 16px;border-radius:20px;cursor:pointer;margin-right:10px}
.symptom-area{display:flex;flex-direction:column;gap:10px}
.symptom-input-group{display:flex;gap:15px;flex-wrap:wrap}
.symptom-input{flex:2;min-width:300px;height:100px;padding:10px;border:2px solid #e0e0e0;border-radius:10px;font-family:inherit;resize:vertical}
.file-list{flex:1;min-width:200px;background:#f8f9fa;border-radius:10px;padding:10px;max-height:150px;overflow-y:auto}
.file-list-title{font-size:12px;font-weight:bold;color:#667eea;margin-bottom:8px}
.file-item{font-size:11px;padding:4px 8px;margin:2px 0;background:white;border-radius:5px;cursor:pointer;display:flex;justify-content:space-between;align-items:center}
.file-item:hover{background:#e9ecef}
.button-group{display:flex;gap:10px;margin-top:10px;flex-wrap:wrap}
.modal{display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.5);justify-content:center;align-items:center;z-index:1000}
.modal-content{background:white;padding:20px;border-radius:15px;width:90%;max-width:500px;position:relative}
.modal-close{position:absolute;top:10px;right:15px;font-size:24px;cursor:pointer;color:#999}
.modal-close:hover{color:#333}
.temp-select{display:none;margin-top:5px}
.temp-select select{padding:5px;border-radius:5px}
.voice-options{display:none;margin-top:5px;gap:10px}
.voice-options button{background:#28a745;color:white;border:none;padding:5px 10px;border-radius:15px;cursor:pointer}
.print-options{display:none;margin-top:5px}
.print-options label{display:block;margin:5px 0}
.delete-file{color:#dc3545;cursor:pointer;margin-left:8px}
</style>
</head>
<body>
<div class="header"><h2>🏥 Личный кабинет пациента</h2><a href="/">Выйти</a></div>
<div id="patientInfo" class="section"></div>

<div class="section"><div class="section-title">📝 Добавить симптом</div>
<div class="symptom-area"><div class="symptom-input-group"><textarea id="symptomText" class="symptom-input" placeholder="Опишите симптом..."></textarea><div class="file-list"><div class="file-list-title">📎 Загруженные файлы</div><div id="uploadedFilesList"></div></div></div>
<div class="button-group"><div class="temp-btn-container"><button class="temp-btn" onclick="toggleTempSelect()">🌡️ Добавить температуру</button><div id="tempSelect" class="temp-select"><select id="tempValue"><option>36.0</option><option>36.1</option><option>36.2</option><option>36.3</option><option>36.4</option><option>36.5</option><option selected>36.6</option><option>36.7</option><option>36.8</option><option>36.9</option><option>37.0</option><option>37.1</option><option>37.2</option><option>37.3</option><option>37.4</option><option>37.5</option><option>38.0</option><option>38.5</option><option>39.0</option></select><button onclick="addSelectedTemp()">OK</button></div></div>
<div class="voice-btn-container"><button class="voice-btn" onclick="toggleVoiceOptions()">🎤 Голосовой ввод</button><div id="voiceOptions" class="voice-options"><button onclick="startLiveVoice()">🎙️ Запись с микрофона</button><button onclick="uploadVoiceFile()">📁 Загрузить аудиофайл</button></div></div>
<button class="photo-btn" onclick="uploadFileForSymptom()">📸 Загрузить фото</button>
<button onclick="addSymptom()" style="background:#667eea;color:white;border:none;padding:8px 16px;border-radius:20px;cursor:pointer">💾 Сохранить симптом</button>
<div class="print-btn-container"><button class="btn-add" onclick="togglePrintOptions()">🖨️ Распечатать</button><div id="printOptions" class="print-options"><label><input type="checkbox" id="printSymptoms"> Симптомы</label><label><input type="checkbox" id="printAllergies"> Аллергии</label><label><input type="checkbox" id="printVaccinations"> Вакцинации</label><label><input type="checkbox" id="printMedications"> Препараты</label><label><input type="checkbox" id="printDiagnoses"> Диагнозы</label><button onclick="printSelected()">Печать</button></div></div></div></div>
<div id="symptomsList" class="item-list" style="margin-top:15px"></div></div>

<div class="section"><div class="section-title">⚠️ Аллергии <button class="btn-add" onclick="showAddAllergy()">+ Добавить</button></div>
<div id="allergiesList" class="item-list"></div></div>

<div class="section"><div class="section-title">💉 Вакцинации <button class="btn-add" onclick="showAddVaccination()">+ Добавить</button></div>
<div id="vaccinationsList" class="item-list"></div></div>

<div class="section"><div class="section-title">💊 Мои препараты <button class="btn-add" onclick="showAddMedication()">+ Добавить</button></div>
<div id="medicationsList" class="item-list"></div></div>

<div class="section"><div class="section-title">📋 Диагнозы</div>
<div id="diagnosesList" class="item-list"></div></div>

<!-- Модальные окна -->
<div id="allergyModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('allergyModal')">&times;</span><h3>Добавить аллергию</h3><input type="text" id="allergenName" placeholder="Аллерген" style="width:100%;margin:10px 0;padding:8px"><input type="text" id="allergenReaction" placeholder="Реакция" style="width:100%;margin:10px 0;padding:8px"><button onclick="addAllergy()">Сохранить</button></div></div>

<div id="vaccinationModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('vaccinationModal')">&times;</span><h3>Добавить вакцинацию</h3><input type="text" id="vaccineName" placeholder="Название" style="width:100%;margin:10px 0;padding:8px"><input type="date" id="vaccineDate" style="width:100%;margin:10px 0;padding:8px"><button onclick="addVaccination()">Сохранить</button></div></div>

<div id="medicationModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('medicationModal')">&times;</span><h3>Добавить препарат</h3><input type="text" id="medName" placeholder="Название" style="width:100%;margin:10px 0;padding:8px"><input type="text" id="medDosage" placeholder="Дозировка" style="width:100%;margin:10px 0;padding:8px"><div id="medTimesList"></div><button type="button" onclick="addMedTimeField()">+ Добавить время</button><input type="date" id="medStartDate" style="width:100%;margin:10px 0;padding:8px"><select id="medDuration" style="width:100%;margin:10px 0;padding:8px"><option value="3 дня">3 дня</option><option value="5 дней">5 дней</option><option value="7 дней" selected>7 дней</option><option value="10 дней">10 дней</option><option value="14 дней">14 дней</option><option value="Постоянно">Постоянно</option></select><button onclick="addMedication()">Сохранить</button></div></div>

<div id="weightHistoryModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('weightHistoryModal')">&times;</span><h3>История изменения веса</h3><div id="weightHistoryList"></div></div></div>

<div id="medicationHistoryModal" class="modal"><div class="modal-content"><span class="modal-close" onclick="closeModal('medicationHistoryModal')">&times;</span><h3>История приема препарата</h3><div id="medicationHistoryList"></div></div></div>

<script>
const patientId = "` + patientId + `";
let currentEditMedId = null;

async function loadAllData() {
    await loadPatient();
    await loadAllergies();
    await loadVaccinations();
    await loadSymptoms();
    await loadMedications();
    await loadDiagnoses();
    await loadUploadedFiles();
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
        '<div class="info-item"><div class="info-label">ФИО</div><div class="info-value">' + p.full_name + '</div></div>' +
        '<div class="info-item"><div class="info-label">Дата рождения</div><div class="info-value">' + new Date(p.birth_date).toLocaleDateString() + '</div></div>' +
        '<div class="info-item"><div class="info-label">Рост/Вес</div><div class="info-value">' + p.height + ' см / ' + p.weight + ' ' + p.weight_unit + 
            '<button class="btn-icon" onclick="editWeight()">✏️</button><button class="btn-icon" onclick="showWeightHistory()">📊</button></div></div>' +
        '<div class="info-item"><div class="info-label">ИМТ</div><div class="info-value">' + bmi + ' (' + cat + ')</div></div></div>';
}

async function editWeight() {
    const h = prompt('Рост (см):', document.querySelector('#weightValue')?.innerText.split(' ')[0] || '175');
    const w = prompt('Вес:', document.querySelector('#weightValue')?.innerText.split(' ')[2] || '70');
    if (h && w) {
        await fetch('/api/patient/' + patientId + '/weight-history', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ height: parseInt(h), weight: parseFloat(w), weight_unit: 'кг' })
        });
        loadPatient();
        alert('Данные обновлены!');
    }
}

async function showWeightHistory() {
    const res = await fetch('/api/patient/' + patientId + '/weight-history');
    const items = await res.json();
    const html = '<table style="width:100%;border-collapse:collapse">么多的<th>Дата</th><th>Рост</th><th>Вес</th></tr>' +
        items.map(w => '<tr><td>' + new Date(w.recorded_at).toLocaleString() + '</td><td>' + w.height + ' см</td><td>' + w.weight + ' ' + w.weight_unit + '</td></tr>').join('') +
        '</table>';
    document.getElementById('weightHistoryList').innerHTML = html;
    document.getElementById('weightHistoryModal').style.display = 'flex';
}

async function loadAllergies() {
    const res = await fetch('/api/allergies/' + patientId);
    const items = await res.json();
    const c = document.getElementById('allergiesList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет аллергий</div>';
    else c.innerHTML = items.map(a => '<div class="item">⚠️ ' + a.allergen + ' - ' + a.reaction + '<button class="btn-delete" onclick="deleteAllergy(\'' + a.id + '\')">🗑️</button></div>').join('');
}

async function loadVaccinations() {
    const res = await fetch('/api/vaccinations/' + patientId);
    const items = await res.json();
    const c = document.getElementById('vaccinationsList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет вакцинаций</div>';
    else c.innerHTML = items.map(v => '<div class="item">💉 ' + v.name + ' - ' + new Date(v.date).toLocaleDateString() + '<button class="btn-delete" onclick="deleteVaccination(\'' + v.id + '\')">🗑️</button></div>').join('');
}

async function loadSymptoms() {
    const res = await fetch('/api/symptoms/' + patientId);
    const items = await res.json();
    const c = document.getElementById('symptomsList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет симптомов</div>';
    else c.innerHTML = items.map(s => '<div class="item">📝 ' + s.text + '<button class="btn-delete" onclick="deleteSymptom(\'' + s.id + '\')">🗑️</button></div>').join('');
}

async function loadMedications() {
    const res = await fetch('/api/medications/' + patientId);
    const items = await res.json();
    const c = document.getElementById('medicationsList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет препаратов</div>';
    else c.innerHTML = items.filter(m => m.status === 'active').map(m => {
        const startDate = new Date(m.start_date).toLocaleDateString();
        const now = new Date();
        return '<div class="item-med"><strong>💊 ' + m.name + ' ' + m.dosage + '</strong>' +
            '<div class="med-times">' + (m.times || []).map(t => {
                let cls = '';
                if (m.taken_logs && m.taken_logs[t]) cls = 'taken';
                else {
                    const [h, mm] = t.split(':');
                    const mt = new Date();
                    mt.setHours(parseInt(h), parseInt(mm), 0);
                    if (mt < now) cls = 'missed';
                }
                return '<span class="time-badge ' + cls + '" onclick="markTaken(\'' + m.id + '\',\'' + t + '\')">🕐 ' + t + '</span>';
            }).join('') + '</div>' +
            '<small>Начало: ' + startDate + '</small>' +
            '<div><button class="btn-edit" onclick="editMedication(\'' + m.id + '\')">✏️ Ред.</button>' +
            '<button class="btn-icon" onclick="showMedicationHistory(\'' + m.id + '\')">📊</button>' +
            '<button class="btn-delete" onclick="deleteMedication(\'' + m.id + '\')">🗑️ Удалить</button></div></div>';
    }).join('');
}

async function showMedicationHistory(medId) {
    const res = await fetch('/api/medications/' + patientId + '/' + medId + '/history');
    const logs = await res.json();
    const html = '<table style="width:100%;border-collapse:collapse"><tr><th>Дата</th><th>Время</th><th>Статус</th></tr>' +
        logs.map(l => '<tr><td>' + new Date(l.scheduled_time).toLocaleDateString() + '</td><td>' + new Date(l.scheduled_time).toLocaleTimeString() + '</td><td>' + (l.status === 'taken' ? '✅ Принят' : '❌ Пропущен') + '</td></tr>').join('') +
        '</table>';
    document.getElementById('medicationHistoryList').innerHTML = html;
    document.getElementById('medicationHistoryModal').style.display = 'flex';
}

async function loadDiagnoses() {
    const res = await fetch('/api/diagnoses/' + patientId);
    const items = await res.json();
    const c = document.getElementById('diagnosesList');
    if (items.length === 0) c.innerHTML = '<div class="item">Нет диагнозов</div>';
    else c.innerHTML = items.map(d => '<div class="item">📋 ' + d.name + ' - ' + new Date(d.date).toLocaleDateString() +
        '<div class="medication-list">' + (d.medications || []).map(m => '<div>💊 ' + m + '</div>').join('') +
        '<button class="btn-add" onclick="addMedToDiagnosis(\'' + d.id + '\')">+ Добавить препарат</button></div></div>').join('');
}

async function addMedToDiagnosis(diaId) {
    const med = prompt('Название препарата:');
    if (med) {
        const dia = window.diagnosesData?.find(d => d.id === diaId);
        const meds = dia ? dia.medications : [];
        meds.push(med);
        await fetch('/api/diagnoses/' + patientId + '/' + diaId + '/medications', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ medications: meds })
        });
        loadDiagnoses();
    }
}

async function loadUploadedFiles() {
    const res = await fetch('/api/uploads/' + patientId);
    const files = await res.json();
    const container = document.getElementById('uploadedFilesList');
    if (files.length === 0) container.innerHTML = '<div class="file-item">Нет файлов</div>';
    else container.innerHTML = files.map(f => '<div class="file-item" onclick="viewFile(\'' + f.path + '\')">📄 ' + f.name.substring(0,30) + '<span class="delete-file" onclick="event.stopPropagation();deleteFile(\'' + f.id + '\')">❌</span></div>').join('');
}

function viewFile(path) {
    window.open('/uploads/' + path, '_blank');
}

async function deleteFile(fileId) {
    if (confirm('Удалить файл?')) {
        await fetch('/api/uploads/' + patientId + '/' + fileId, { method: 'DELETE' });
        loadUploadedFiles();
    }
}

function toggleTempSelect() {
    const sel = document.getElementById('tempSelect');
    sel.style.display = sel.style.display === 'none' ? 'block' : 'none';
}

function addSelectedTemp() {
    const temp = document.getElementById('tempValue').value;
    const now = new Date();
    const time = now.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
    const date = now.toLocaleDateString('ru-RU');
    const st = document.getElementById('symptomText');
    st.value = st.value + '[' + date + ' ' + time + '] Температура: ' + temp + '°C\n';
    document.getElementById('tempSelect').style.display = 'none';
}

function toggleVoiceOptions() {
    const opt = document.getElementById('voiceOptions');
    opt.style.display = opt.style.display === 'none' ? 'flex' : 'none';
}

function startLiveVoice() {
    if ('webkitSpeechRecognition' in window) {
        const r = new webkitSpeechRecognition();
        r.lang = 'ru-RU';
        r.onresult = e => {
            const text = e.results[0][0].transcript;
            const st = document.getElementById('symptomText');
            st.value = st.value + text + '\n';
        };
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
            const st = document.getElementById('symptomText');
            st.value = st.value + data.text + '\n';
            alert('Аудио распознано!');
        }
    };
    input.click();
    document.getElementById('voiceOptions').style.display = 'none';
}

function uploadFileForSymptom() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*,application/pdf';
    input.onchange = async e => {
        const fd = new FormData();
        fd.append('file', e.target.files[0]);
        fd.append('patientId', patientId);
        const res = await fetch('/api/upload', { method: 'POST', body: fd });
        const data = await res.json();
        if (data.success) {
            loadUploadedFiles();
            alert('Файл загружен!');
        }
    };
    input.click();
}

async function addSymptom() {
    let txt = document.getElementById('symptomText').value;
    if (!txt) { alert('Введите симптом'); return; }
    const now = new Date();
    const dateStr = now.toLocaleDateString('ru-RU') + ', ' + now.toLocaleTimeString('ru-RU');
    txt = dateStr + ' - ' + txt;
    await fetch('/api/symptoms/' + patientId, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: txt })
    });
    document.getElementById('symptomText').value = '';
    loadSymptoms();
}

function togglePrintOptions() {
    const opt = document.getElementById('printOptions');
    opt.style.display = opt.style.display === 'none' ? 'block' : 'none';
}

function printSelected() {
    let content = '';
    if (document.getElementById('printSymptoms')?.checked) {
        const symptoms = document.querySelectorAll('#symptomsList .item');
        symptoms.forEach(s => { content += s.innerText + '\n\n'; });
    }
    if (document.getElementById('printAllergies')?.checked) {
        const allergies = document.querySelectorAll('#allergiesList .item');
        allergies.forEach(a => { content += a.innerText + '\n\n'; });
    }
    if (document.getElementById('printVaccinations')?.checked) {
        const vaccs = document.querySelectorAll('#vaccinationsList .item');
        vaccs.forEach(v => { content += v.innerText + '\n\n'; });
    }
    if (document.getElementById('printMedications')?.checked) {
        const meds = document.querySelectorAll('#medicationsList .item-med');
        meds.forEach(m => { content += m.innerText + '\n\n'; });
    }
    if (document.getElementById('printDiagnoses')?.checked) {
        const diags = document.querySelectorAll('#diagnosesList .item');
        diags.forEach(d => { content += d.innerText + '\n\n'; });
    }
    const win = window.open('', '', 'width=800,height=600');
    win.document.write('<html><head><title>Печать</title></head><body><pre>' + content + '</pre></body></html>');
    win.document.close();
    win.print();
}

async function deleteSymptom(id) {
    if (confirm('Удалить симптом?')) await fetch('/api/symptoms/' + patientId + '/' + id, { method: 'DELETE' });
    loadSymptoms();
}

function showAddAllergy() { document.getElementById('allergyModal').style.display = 'flex'; }
async function addAllergy() {
    const data = { allergen: document.getElementById('allergenName').value, reaction: document.getElementById('allergenReaction').value };
    await fetch('/api/allergies/' + patientId, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    closeModal('allergyModal');
    loadAllergies();
}
async function deleteAllergy(id) {
    if (confirm('Удалить аллергию?')) await fetch('/api/allergies/' + patientId + '/' + id, { method: 'DELETE' });
    loadAllergies();
}

function showAddVaccination() { document.getElementById('vaccinationModal').style.display = 'flex'; }
async function addVaccination() {
    const data = { name: document.getElementById('vaccineName').value, date: document.getElementById('vaccineDate').value };
    await fetch('/api/vaccinations/' + patientId, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    closeModal('vaccinationModal');
    loadVaccinations();
}
async function deleteVaccination(id) {
    if (confirm('Удалить вакцинацию?')) await fetch('/api/vaccinations/' + patientId + '/' + id, { method: 'DELETE' });
    loadVaccinations();
}

function addMedTimeField() {
    const c = document.getElementById('medTimesList');
    const d = document.createElement('div');
    d.innerHTML = '<input type="time" class="med-time" style="margin:5px;padding:5px"> <button type="button" onclick="this.parentElement.remove()">❌</button>';
    c.appendChild(d);
}

function showAddMedication() {
    currentEditMedId = null;
    document.getElementById('medTimesList').innerHTML = '';
    document.getElementById('medName').value = '';
    document.getElementById('medDosage').value = '';
    document.getElementById('medStartDate').value = '';
    document.getElementById('medicationModal').style.display = 'flex';
}

async function addMedication() {
    const times = Array.from(document.querySelectorAll('.med-time')).map(t => t.value);
    const data = {
        name: document.getElementById('medName').value,
        dosage: document.getElementById('medDosage').value,
        times: times,
        start_date: document.getElementById('medStartDate').value,
        duration: document.getElementById('medDuration').value,
        frequency: times.length + ' раз в день',
        status: 'active'
    };
    await fetch('/api/medications/' + patientId, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) });
    closeModal('medicationModal');
    loadMedications();
}

async function editMedication(id) {
    alert('Редактирование препарата будет в следующей версии');
}

async function markTaken(medId, time) {
    await fetch('/api/medications/' + patientId + '/' + medId + '/taken', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ time: time, taken_at: new Date().toISOString() })
    });
    loadMedications();
}

async function deleteMedication(id) {
    if (confirm('Удалить препарат?')) await fetch('/api/medications/' + patientId + '/' + id, { method: 'DELETE' });
    loadMedications();
}

function closeModal(id) { document.getElementById(id).style.display = 'none'; }

loadAllData();
</script>
</body>
</html>`)
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

func markTaken(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    medId := vars["medId"]
    var req struct{ Time, TakenAt string }
    json.NewDecoder(r.Body).Decode(&req)
    mu.Lock()
    items := medications[patientId]
    for i, m := range items {
        if m.ID == medId {
            if m.TakenLogs == nil {
                m.TakenLogs = make(map[string]string)
            }
            m.TakenLogs[req.Time] = req.TakenAt
            medications[patientId][i] = m
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getMedicationHistory(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    medId := vars["medId"]
    mu.RLock()
    var logs []map[string]interface{}
    if items, ok := medications[vars["id"]]; ok {
        for _, m := range items {
            if m.ID == medId {
                for t, takenAt := range m.TakenLogs {
                    logs = append(logs, map[string]interface{}{
                        "scheduled_time": t,
                        "taken_at":       takenAt,
                        "status":         "taken",
                    })
                }
                break
            }
        }
    }
    mu.RUnlock()
    sendJSON(w, logs, http.StatusOK)
}

func getDiagnoses(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    mu.RLock()
    items := diagnoses[vars["id"]]
    mu.RUnlock()
    sendJSON(w, items, http.StatusOK)
}

func updateDiagnosisMedications(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    patientId := vars["id"]
    diaId := vars["diaId"]
    var req struct{ Medications []string }
    json.NewDecoder(r.Body).Decode(&req)
    mu.Lock()
    items := diagnoses[patientId]
    for i, d := range items {
        if d.ID == diaId {
            items[i].Medications = req.Medications
            diagnoses[patientId] = items
            break
        }
    }
    mu.Unlock()
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func getLabResults(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, []LabResult{}, http.StatusOK)
}

func getMedicalRecords(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, []MedicalRecord{}, http.StatusOK)
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
    sendJSON(w, map[string]interface{}{"success": true, "text": "Головная боль"}, http.StatusOK)
}

func voiceUploadHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(32 << 20)
    sendJSON(w, map[string]interface{}{"success": true, "text": "Распознанный текст из аудиофайла"}, http.StatusOK)
}

func printHandler(w http.ResponseWriter, r *http.Request) {
    sendJSON(w, map[string]interface{}{"success": true}, http.StatusOK)
}

func sendJSON(w http.ResponseWriter, data interface{}, status int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}
