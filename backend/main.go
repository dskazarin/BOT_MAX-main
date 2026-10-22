package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

type Guideline struct {
	ID                         string                `json:"id"`
	Version                    string                `json:"version"`
	Specialty                  string                `json:"specialty"`
	Title                      string                `json:"title"`
	DiagnosticCriteria         DiagnosticCriteria    `json:"diagnostic_criteria"`
	BacterialCriteria          BacterialCriteria     `json:"bacterial_criteria"`
	AntibioticIndications      AntibioticIndications `json:"antibiotic_indications"`
	Antibiotics                Antibiotics           `json:"antibiotics"`
	Duration                   Duration              `json:"duration"`
	LocalTherapy               LocalTherapy          `json:"local_therapy"`
	Symptomatic                Symptomatic           `json:"symptomatic"`
	SurgeryIndications         []SurgeryIndication   `json:"surgery_indications"`
	HospitalizationIndications []string              `json:"hospitalization_indications"`
	DifferentialDiagnosis      DifferentialDiagnosis `json:"differential_diagnosis"`
	Diagnostics                Diagnostics           `json:"diagnostics"`
}

type DiagnosticCriteria struct {
	Adult Criteria `json:"adult"`
	Child Criteria `json:"child"`
}

type Criteria struct {
	RequiredSymptoms []string `json:"required_symptoms"`
	OptionalSymptoms []string `json:"optional_symptoms"`
	MinSymptomsCount int      `json:"min_symptoms_count"`
	MaxDurationDays  int      `json:"max_duration_days"`
}

type BacterialCriteria struct {
	MinSigns int      `json:"min_signs"`
	Signs    []string `json:"signs"`
}

type AntibioticIndications struct {
	Adult IndicationSet `json:"adult"`
	Child IndicationSet `json:"child"`
}

type IndicationSet struct {
	Always  []string    `json:"always"`
	When    []Condition `json:"when"`
	Exclude []Condition `json:"exclude"`
}

type Condition struct {
	Condition string `json:"condition"`
	Reason    string `json:"reason"`
}

type Antibiotics struct {
	Adult AntibioticScheme `json:"adult"`
	Child AntibioticScheme `json:"child"`
}

type AntibioticScheme struct {
	FirstLine    []Antibiotic `json:"first_line"`
	Alternatives []Antibiotic `json:"alternatives"`
}

type Antibiotic struct {
	Name       string            `json:"name"`
	Dosage     string            `json:"dosage"`
	Conditions map[string]string `json:"conditions,omitempty"`
	When       []string          `json:"when,omitempty"`
}

type Duration struct {
	Default     int `json:"default"`
	Complicated int `json:"complicated"`
}

type LocalTherapy struct {
	Decongestants interface{} `json:"decongestants"`
	Irrigation    string      `json:"irrigation"`
	TopicalGCS    TopicalGCS  `json:"topical_gcs"`
}

type TopicalGCS struct {
	Age  int    `json:"age"`
	Drug string `json:"drug"`
	Note string `json:"note"`
}

type Symptomatic struct {
	NSAIDs     interface{} `json:"nsaids"`
	Mucoactive string      `json:"mucoactive"`
	Herbal     string      `json:"herbal"`
}

type SurgeryIndication struct {
	Condition string `json:"condition"`
	Action    string `json:"action"`
}

type DifferentialDiagnosis struct {
	Adult []string `json:"adult"`
	Child []string `json:"child"`
}

type Diagnostics struct {
	Stage1 string          `json:"stage1"`
	Stage2 DiagnosticStage `json:"stage2"`
	Stage3 DiagnosticStage `json:"stage3"`
}

type DiagnosticStage struct {
	Lab          *LabTests     `json:"lab,omitempty"`
	Microbiology *Microbiology `json:"microbiology,omitempty"`
	CT           *ImagingRule  `json:"ct,omitempty"`
	XRay         *ImagingRule  `json:"xray,omitempty"`
	Ultrasound   *ImagingRule  `json:"ultrasound,omitempty"`
}

type LabTests struct {
	When  []string `json:"when"`
	Tests []string `json:"tests"`
}

type Microbiology struct {
	When     []string `json:"when"`
	Material string   `json:"material"`
}

type ImagingRule struct {
	When []string `json:"when"`
	Note string   `json:"note,omitempty"`
}

type PatientData struct {
	Age              int             `json:"age"`
	IsAdult          bool            `json:"isAdult"`
	Days             int             `json:"days"`
	Temp             float64         `json:"temp"`
	Leikocytoz       float64         `json:"leikocytoz"`
	EpizodovVGod     int             `json:"epizodovVGod"`
	Immunodeficit    bool            `json:"immunodeficit"`
	Comorbida        bool            `json:"comorbida"`
	Allergiya        bool            `json:"allergiya"`
	Symptoms         map[string]bool `json:"symptoms"`
	BacterialSigns   map[string]bool `json:"bacterialSigns"`
	Gnoinye          bool            `json:"gnoinye"`
	VtorayaVolna     bool            `json:"vtorayaVolna"`
	Vnutricherepnye  bool            `json:"vnutricherepnye"`
	OrbitalnyeOslozh bool            `json:"orbitalnyeOslozh"`
}

type Report struct {
	GuidelineID         string            `json:"guidelineId"`
	GuidelineTitle      string            `json:"guidelineTitle"`
	Specialty           string            `json:"specialty"`
	DiagnosisMatch      bool              `json:"diagnosisMatch"`
	DiagnosisReason     string            `json:"diagnosisReason"`
	BacterialSignsCount int               `json:"bacterialSignsCount"`
	AntibioticNeeded    bool              `json:"antibioticNeeded"`
	AntibioticReason    string            `json:"antibioticReason"`
	Severity            string            `json:"severity"`
	Treatment           Treatment         `json:"treatment"`
	Diagnostics         DiagnosticsResult `json:"diagnostics"`
	Hospitalization     bool              `json:"hospitalization"`
	ConfidenceScore     float64           `json:"confidenceScore"`
}

type Treatment struct {
	Antibiotic *AntibioticResult `json:"antibiotic,omitempty"`
	Duration   int               `json:"duration"`
}

type AntibioticResult struct {
	Name   string `json:"name"`
	Dosage string `json:"dosage"`
}

type DiagnosticsResult struct {
	Stage1       string   `json:"stage1"`
	Stage2       string   `json:"stage2"`
	Stage3       string   `json:"stage3"`
	Differential []string `json:"differential"`
}

type Loader struct {
	GuidelinesPath string
	cache          map[string]*Guideline
}

func NewLoader(path string) *Loader {
	return &Loader{GuidelinesPath: path, cache: make(map[string]*Guideline)}
}

func (l *Loader) LoadAll() (map[string]*Guideline, error) {
	result := make(map[string]*Guideline)
	entries, err := os.ReadDir(l.GuidelinesPath)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		specialty := entry.Name()
		activePath := filepath.Join(l.GuidelinesPath, specialty, "active")
		if _, err := os.Stat(activePath); os.IsNotExist(err) {
			continue
		}
		versions, err := os.ReadDir(activePath)
		if err != nil {
			continue
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			gp := filepath.Join(activePath, version.Name(), "guideline.json")
			g, err := l.loadFromFile(gp)
			if err != nil {
				continue
			}
			if g != nil {
				result[g.ID] = g
				l.cache[g.ID] = g
			}
		}
	}
	return result, nil
}

func (l *Loader) loadFromFile(path string) (*Guideline, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var g Guideline
	if err := json.NewDecoder(file).Decode(&g); err != nil {
		return nil, err
	}
	if g.ID == "" {
		g.ID = strings.TrimSuffix(filepath.Base(filepath.Dir(path)), ".json")
	}
	return &g, nil
}

func (l *Loader) GetLatest(specialty string) *Guideline {
	var latest *Guideline
	var maxVersion string
	for _, g := range l.cache {
		if g.Specialty != specialty {
			continue
		}
		if maxVersion == "" || g.Version > maxVersion {
			maxVersion = g.Version
			latest = g
		}
	}
	return latest
}

func countBacterialSigns(p PatientData) int {
	count := 0
	if p.Gnoinye || p.BacterialSigns["gnoinye_vydeleniya"] {
		count++
	}
	if p.Symptoms["golovnaya_bol"] || p.BacterialSigns["golovnaya_bol"] {
		count++
	}
	if p.Temp >= 38.0 || p.BacterialSigns["temp_38_and_above"] {
		count++
	}
	if p.VtorayaVolna || p.BacterialSigns["vtoraya_volna"] {
		count++
	}
	if p.Leikocytoz > 15 || p.BacterialSigns["leikocytoz_15_and_above"] {
		count++
	}
	return count
}

func checkPatient(g *Guideline, p PatientData) *Report {
	r := &Report{
		GuidelineID:     g.ID,
		GuidelineTitle:  g.Title,
		Specialty:       g.Specialty,
		ConfidenceScore: 0.95,
	}

	// Тяжесть
	if p.Vnutricherepnye || p.OrbitalnyeOslozh {
		r.Severity = "тяжелая"
	} else if p.Symptoms["bol_v_litse"] || p.Temp >= 38.0 {
		r.Severity = "средняя"
	} else {
		r.Severity = "легкая"
	}

	// Диагноз
	crit := g.DiagnosticCriteria.Adult
	if !p.IsAdult {
		crit = g.DiagnosticCriteria.Child
	}
	for _, req := range crit.RequiredSymptoms {
		if !p.Symptoms[req] {
			r.DiagnosisMatch = false
			r.DiagnosisReason = "Отсутствует обязательный симптом: " + req
			break
		}
	}
	if r.DiagnosisReason == "" {
		count := 0
		for _, opt := range crit.OptionalSymptoms {
			if p.Symptoms[opt] {
				count++
			}
		}
		if count >= crit.MinSymptomsCount {
			r.DiagnosisMatch = true
			r.DiagnosisReason = "Соответствует критериям ОС"
		} else {
			r.DiagnosisReason = fmt.Sprintf("Менее %d симптомов", crit.MinSymptomsCount)
		}
	}

	// Бактериальные признаки
	r.BacterialSignsCount = countBacterialSigns(p)

	// Антибиотики
	if r.Severity == "тяжелая" || r.Severity == "средняя" || r.BacterialSignsCount >= 3 {
		r.AntibioticNeeded = true
		r.AntibioticReason = "показания по тяжести/бактериальным признакам"
		r.Treatment.Antibiotic = &AntibioticResult{Name: "Амоксициллин", Dosage: "500-1000 мг 3 р/д"}
		r.Treatment.Duration = 7
	} else if p.IsAdult && p.Days >= 5 {
		r.AntibioticNeeded = true
		r.AntibioticReason = "симптомы >= 5-7 дней"
		r.Treatment.Antibiotic = &AntibioticResult{Name: "Амоксициллин", Dosage: "500-1000 мг 3 р/д"}
		r.Treatment.Duration = 7
	} else if p.Days > 10 && r.BacterialSignsCount < 3 {
		r.AntibioticNeeded = false
		r.AntibioticReason = "поствирусный синусит (АБ не показан)"
	} else {
		r.AntibioticNeeded = false
		r.AntibioticReason = "нет показаний для АБ"
	}

	// Диагностика
	r.Diagnostics = DiagnosticsResult{
		Stage1:       "Осмотр врача-оториноларинголога",
		Stage2:       "Не показан",
		Stage3:       "Не показан",
		Differential: []string{"аллергический ринит", "обострение хронического риносинусита"},
	}

	// Госпитализация
	if r.Severity == "тяжелая" {
		r.Hospitalization = true
	}

	return r
}

var loader *Loader
var guidelines map[string]*Guideline
var db *sql.DB

// recoverMiddleware ловит panic в HTTP-хендлерах, логирует и отдаёт 500.
// Защищает сервер от падения при панике в одном запросе.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "method", r.Method, "path", r.URL.Path, "rec", rec, "stack", string(debug.Stack()))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func main() {
	env, errValidate := validateEnv()
	if errValidate != nil {
		slog.Error("env", "err", errValidate)
		os.Exit(1)
	}
	currentEnv = env
	setupLogging(currentEnv)
	slog.Info("env", "name", env.Name, "auth_enforce", env.AuthEnforce, "jwt_secret_len", len(env.JWTSecret))

	wd, _ := os.Getwd()
	rootDir := wd
	if filepath.Base(wd) == "backend" {
		rootDir = filepath.Dir(wd)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "index.html")); os.IsNotExist(err) {
		if _, err := os.Stat("index.html"); err == nil {
			rootDir = "."
		}
	}

	slog.Info("корень проекта", "rootDir", rootDir)

	loader = NewLoader(filepath.Join(rootDir, "guidelines"))
	guidelines, _ = loader.LoadAll()
	slog.Info("загружено рекомендаций", "count", len(guidelines))

	// === SQLite (шаг 1) ===
	var err error
	dbPath := currentEnv.DBPath
	if dbPath == "" {
		dbPath = filepath.Join(rootDir, "bot_max.db")
	}
	db, err = InitDB(dbPath)
	if err != nil {
		slog.Error("InitDB", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	runDevSeeds(db, currentEnv)
	slog.Info("SQLite готова", "dbPath", dbPath)

	// === B19.3: фоновый retention audit_log ===
	// Запускается всегда, но при days<=0 runAuditRetention сразу выходит;
	// при этом db остаётся под защитой defer db.Close() до конца main().
	bgCtx, bgCancel := context.WithCancel(context.Background())
	bgDone := make(chan struct{})
	go func() {
		defer close(bgDone)
		runRetention(bgCtx, db,
			currentEnv.AuditRetentionDays,
			currentEnv.RefreshRetentionDays,
			currentEnv.AuditCleanupInterval)
	}()

	http.HandleFunc("/api/health", handleHealth)

	// B26.1: Prometheus-совместимый /api/metrics.
	registerMetricsRoutes()

	http.HandleFunc("/api/specialties", func(w http.ResponseWriter, r *http.Request) {
		specs := make(map[string][]string)
		for _, g := range guidelines {
			specs[g.Specialty] = append(specs[g.Specialty], g.Version)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(specs)
	})

	http.HandleFunc("/api/check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Specialty string      `json:"specialty"`
			Patient   PatientData `json:"patient"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		g := loader.GetLatest(req.Specialty)
		if g == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		report := checkPatient(g, req.Patient)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	})

	// Роуты для /api/patients — см. backend/patients.go
	registerPatientRoutes()

	// Аутентификация (Шаг 6): POST /api/auth/login
	registerAuthRoutes()
	registerPatientAuthRoutes()
	registerPatientVitalsRoutes()
	registerPatientRecordsRoutes() // B30.5: self-vitals
	registerPatientProfileRoutes() // B30.7: self-profile
	registerConsentRoutes()

	// Impersonation (B20): /api/impersonation/{start,stop}
	registerImpersonationRoutes()

	// CRUD по докторам (B5): /api/doctors, /api/doctors/{id} — только admin/superadmin
	registerDoctorRoutes()

	// Audit-log (B9): /api/audit — только admin/superadmin
	registerAuditRoutes()

	// Encounters (B28): /api/encounters — doctor/admin/superadmin
	registerEncounterRoutes()

	http.Handle("/", http.FileServer(http.Dir(rootDir)))

	listener, err := net.Listen("tcp", ":8082")
	if err != nil {
		slog.Error("порт 8082 занят", "err", err, "hint", "fuser -k 8082/tcp")
		os.Exit(1)
	}

	srv := &http.Server{
		Handler:      recoverMiddleware(authMiddleware(requirePasswordChanged(http.DefaultServeMux))),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Запуск сервера в горутине, чтобы main() мог слушать сигналы
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("сервер запущен", "url", "http://localhost:8082")
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Graceful shutdown по SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		slog.Info("получен сигнал, graceful shutdown", "signal", sig.String())
	case err := <-serverErr:
		slog.Error("http.Serve", "err", err)
		os.Exit(1)
	}

	// === B19.3: останов фонового retention перед srv.Shutdown ===
	bgCancel()
	select {
	case <-bgDone:
		slog.Info("audit retention: goroutine остановлена")
	case <-time.After(3 * time.Second):
		slog.Warn("audit retention: не остановилась за 3s, продолжаем shutdown")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Warn("shutdown error", "err", err)
	} else {
		slog.Info("сервер остановлен")
	}
}

// handleHealth — GET /api/health (B25.1).
// Базовая часть: {"status":"ok"}.
// Если retention включён (audit_days>0 || refresh_days>0) — добавляется
// секция retention с last_run_at, счётчиками и healthy-флагом.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	resp := map[string]any{"status": "ok"}

	if currentEnv != nil &&
		(currentEnv.AuditRetentionDays > 0 || currentEnv.RefreshRetentionDays > 0) {
		st := getRetentionStatus()
		healthy := retentionHealthy(
			currentEnv.AuditRetentionDays,
			currentEnv.RefreshRetentionDays,
			currentEnv.AuditCleanupInterval,
		)
		sec := map[string]any{
			"enabled":         true,
			"healthy":         healthy,
			"audit_days":      currentEnv.AuditRetentionDays,
			"refresh_days":    currentEnv.RefreshRetentionDays,
			"audit_deleted":   st.AuditDeleted,
			"refresh_deleted": st.RefreshDeleted,
		}
		if !st.LastRunAt.IsZero() {
			sec["last_run_at"] = st.LastRunAt.UTC().Format(time.RFC3339)
			sec["seconds_since_last_run"] = int64(time.Since(st.LastRunAt).Seconds())
		}
		if st.LastError != "" {
			sec["last_error"] = st.LastError
		}
		resp["retention"] = sec
	}

	_ = json.NewEncoder(w).Encode(resp)
}
