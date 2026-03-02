package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
)

// RouteConfig = konfigurasi untuk 1 domain/service
type RouteConfig struct {
	ID             string            `json:"id"`                       // Unique ID
	Domain         string            `json:"domain"`                   // nama domain, contoh: app.example.com
	ServiceURL     string            `json:"service_url"`              // target backend URL
	EntryPoints    []string          `json:"entry_points,omitempty"`   // default: web/websecure
	TLS            bool              `json:"tls"`                      // pakai HTTPS atau tidak
	Middlewares    []string          `json:"middlewares,omitempty"`    // daftar middleware (opsional)
	PassHostHeader bool              `json:"pass_host_header"`         // forward host header
	CustomHeaders  map[string]string `json:"custom_headers,omitempty"` //custom header tambahan
	PathPrefix     string            `json:"path_prefix,omitempty"`    // jika ingin path match, contoh /api/
}

// TraefikConfig represents the complete Traefik configuration
type TraefikConfig struct {
	HTTP HTTPConfig `yaml:"http" json:"http"`
}

type HTTPConfig struct {
	Routers     map[string]Router     `yaml:"routers" json:"routers"`
	Middlewares map[string]Middleware `yaml:"middlewares,omitempty" json:"middlewares,omitempty"`
	Services    map[string]Service    `yaml:"services" json:"services"`
}

type Router struct {
	Rule        string                 `yaml:"rule" json:"rule"`
	Service     string                 `yaml:"service" json:"service"`
	EntryPoints []string               `yaml:"entryPoints" json:"entryPoints"`
	Middlewares []string               `yaml:"middlewares,omitempty" json:"middlewares,omitempty"`
	TLS         map[string]interface{} `yaml:"tls,omitempty" json:"tls,omitempty"`
}

type Middleware struct {
	RedirectScheme *RedirectScheme `yaml:"redirectScheme,omitempty" json:"redirectScheme,omitempty"`
	Headers        *Headers        `yaml:"headers,omitempty" json:"headers,omitempty"`
}

type RedirectScheme struct {
	Scheme string `yaml:"scheme" json:"scheme"`
}

type Headers struct {
	FrameDeny               bool              `yaml:"frameDeny,omitempty" json:"frameDeny,omitempty"`
	SSLRedirect             bool              `yaml:"sslRedirect,omitempty" json:"sslRedirect,omitempty"`
	BrowserXSSFilter        bool              `yaml:"browserXssFilter,omitempty" json:"browserXssFilter,omitempty"`
	ForceSTSHeader          bool              `yaml:"forceSTSHeader,omitempty" json:"forceSTSHeader,omitempty"`
	STSIncludeSubdomains    bool              `yaml:"stsIncludeSubdomains,omitempty" json:"stsIncludeSubdomains,omitempty"`
	STSPreload              bool              `yaml:"stsPreload,omitempty" json:"stsPreload,omitempty"`
	STSSeconds              int64             `yaml:"stsSeconds,omitempty" json:"stsSeconds,omitempty"`
	CustomFrameOptionsValue string            `yaml:"customFrameOptionsValue,omitempty" json:"customFrameOptionsValue,omitempty"`
	CustomRequestHeaders    map[string]string `yaml:"customRequestHeaders,omitempty" json:"customRequestHeaders,omitempty"`
}

type Service struct {
	LoadBalancer LoadBalancer `yaml:"loadBalancer" json:"loadBalancer"`
}

type LoadBalancer struct {
	Servers        []Server `yaml:"servers" json:"servers"`
	PassHostHeader *bool    `yaml:"passHostHeader,omitempty" json:"passHostHeader,omitempty"`
}

type Server struct {
	URL string `yaml:"url" json:"url"`
}

// Storage handles in-memory and file-based storage
type Storage struct {
	routes       map[string]RouteConfig // penyimpanan route di memory
	mu           sync.RWMutex           // agar aman untuk read/write concurrent
	filePath     string                 // lokasi file routes.json
	outputFormat string                 // yaml/json
}

var storage *Storage

// NewStorage() → inisialisasi storage + baca file routes.json jika ada
func NewStorage(filePath string, outputFormat string) *Storage {
	if outputFormat == "" {
		outputFormat = "yaml"
	}
	s := &Storage{
		routes:       make(map[string]RouteConfig),
		filePath:     filePath,
		outputFormat: strings.ToLower(outputFormat),
	}
	s.loadFromFile() // baca data dari file sebelumnya
	return s
}

// loadFromFile() → baca file JSON dan simpan ke memory
func (s *Storage) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return nil // jika file belum ada, abaikan
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, &s.routes) // convert JSON ke map
}

func (s *Storage) saveToFile() error { // saveToFile() → simpan seluruh routes ke file JSON
	data, err := json.MarshalIndent(s.routes, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

// generateTraefikConfig() → hasilkan traefik-dynamic.yaml/json
func (s *Storage) generateTraefikConfig() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	config := TraefikConfig{
		HTTP: HTTPConfig{
			Routers:     make(map[string]Router),
			Middlewares: make(map[string]Middleware),
			Services:    make(map[string]Service),
		},
	}

	// Tambahkan middleware global default
	config.HTTP.Middlewares["https-redirect"] = Middleware{
		RedirectScheme: &RedirectScheme{
			Scheme: "https",
		},
	}

	config.HTTP.Middlewares["default-headers"] = Middleware{
		Headers: &Headers{
			FrameDeny:               true,
			SSLRedirect:             true,
			BrowserXSSFilter:        true,
			ForceSTSHeader:          true,
			STSIncludeSubdomains:    true,
			STSPreload:              true,
			STSSeconds:              15552000,
			CustomFrameOptionsValue: "SAMEORIGIN",
			CustomRequestHeaders: map[string]string{
				"X-Forwarded-Proto": "https",
			},
		},
	}

	// Loop tiap route untuk buat router dan service
	for id, route := range s.routes {
		serviceName := id

		// Build rule
		rule := fmt.Sprintf("Host(`%s`)", route.Domain)
		if route.PathPrefix != "" {
			rule += fmt.Sprintf(" && PathPrefix(`%s`)", route.PathPrefix)
		}

		// Set default entry points
		entryPoints := route.EntryPoints
		if len(entryPoints) == 0 {
			if route.TLS {
				entryPoints = []string{"websecure"}
			} else {
				entryPoints = []string{"web"}
			}
		}

		// Create router
		router := Router{
			Rule:        rule,
			Service:     serviceName,
			EntryPoints: entryPoints,
		}

		// Add middlewares
		if len(route.Middlewares) > 0 {
			router.Middlewares = route.Middlewares
		}

		// Add TLS if enabled
		if route.TLS {
			router.TLS = make(map[string]interface{})
		}

		config.HTTP.Routers[id] = router

		// Add custom middleware if custom headers exist
		if len(route.CustomHeaders) > 0 {
			middlewareName := fmt.Sprintf("custom-headers-%s", id)
			config.HTTP.Middlewares[middlewareName] = Middleware{
				Headers: &Headers{
					CustomRequestHeaders: route.CustomHeaders,
				},
			}
		}

		// Definisikan backend service
		loadBalancer := LoadBalancer{
			Servers: []Server{
				{URL: route.ServiceURL},
			},
		}

		// Set passHostHeader if specified
		if route.PassHostHeader {
			passHostHeader := true
			loadBalancer.PassHostHeader = &passHostHeader
		}

		config.HTTP.Services[serviceName] = Service{
			LoadBalancer: loadBalancer,
		}
	}

	// Simpan file YAML / JSON hasil generate
	var data []byte
	var err error
	var configPath string

	if s.outputFormat == "yaml" || s.outputFormat == "yml" {
		configPath = filepath.Join(filepath.Dir(s.filePath), "dev-caliana.yaml")
		data, err = yaml.Marshal(config)
	} else {
		configPath = filepath.Join(filepath.Dir(s.filePath), "dev-caliana.json")
		data, err = json.MarshalIndent(config, "", "  ")
	}

	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// API Handlers

func createRoute(w http.ResponseWriter, r *http.Request) {
	var route RouteConfig
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	// Validasi field wajib
	if route.ID == "" || route.Domain == "" || route.ServiceURL == "" {
		respondError(w, http.StatusBadRequest, "ID, Domain, and ServiceURL are required")
		return
	}
	// Simpan ke memory
	storage.mu.Lock()
	if _, exists := storage.routes[route.ID]; exists {
		storage.mu.Unlock()
		respondError(w, http.StatusConflict, "Route with this ID already exists")
		return
	}
	storage.routes[route.ID] = route
	storage.mu.Unlock()
	// Simpan ke file dan regenerasi YAML
	if err := storage.saveToFile(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save route")
		return
	}

	if err := storage.generateTraefikConfig(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate Traefik config")
		return
	}

	respondJSON(w, http.StatusCreated, route)
}

func getRoutes(w http.ResponseWriter, r *http.Request) {
	storage.mu.RLock()
	defer storage.mu.RUnlock()

	routes := make([]RouteConfig, 0, len(storage.routes))
	for _, route := range storage.routes {
		routes = append(routes, route)
	}

	respondJSON(w, http.StatusOK, routes)
}

func getRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	storage.mu.RLock()
	route, exists := storage.routes[id]
	storage.mu.RUnlock()

	if !exists {
		respondError(w, http.StatusNotFound, "Route not found")
		return
	}

	respondJSON(w, http.StatusOK, route)
}

func updateRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var route RouteConfig
	if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	storage.mu.Lock()
	if _, exists := storage.routes[id]; !exists {
		storage.mu.Unlock()
		respondError(w, http.StatusNotFound, "Route not found")
		return
	}

	route.ID = id
	storage.routes[id] = route
	storage.mu.Unlock()

	if err := storage.saveToFile(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save route")
		return
	}

	if err := storage.generateTraefikConfig(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate Traefik config")
		return
	}

	respondJSON(w, http.StatusOK, route)
}

func deleteRoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	storage.mu.Lock()
	if _, exists := storage.routes[id]; !exists {
		storage.mu.Unlock()
		respondError(w, http.StatusNotFound, "Route not found")
		return
	}

	delete(storage.routes, id)
	storage.mu.Unlock()

	if err := storage.saveToFile(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save changes")
		return
	}

	if err := storage.generateTraefikConfig(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate Traefik config")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Route deleted successfully"})
}

func regenerateConfig(w http.ResponseWriter, r *http.Request) {
	if err := storage.generateTraefikConfig(); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate Traefik config")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Traefik config regenerated successfully"})
}

// Helper functions

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	outputFormat := os.Getenv("OUTPUT_FORMAT")
	if outputFormat == "" {
		outputFormat = "yaml"
	}
	// Inisialisasi storage
	storage = NewStorage(filepath.Join(dataDir, "routes.json"), outputFormat)
	// Router mux
	r := mux.NewRouter()
	r.Use(loggingMiddleware)
	// Definisi endpoint
	api := r.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/routes", createRoute).Methods("POST")
	api.HandleFunc("/routes", getRoutes).Methods("GET")
	api.HandleFunc("/routes/{id}", getRoute).Methods("GET")
	api.HandleFunc("/routes/{id}", updateRoute).Methods("PUT")
	api.HandleFunc("/routes/{id}", deleteRoute).Methods("DELETE")
	api.HandleFunc("/regenerate", regenerateConfig).Methods("POST")
	// Endpoint health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	}).Methods("GET")
	// Jalankan server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Traefik Config API on port %s", port)
	log.Printf("Data directory: %s", dataDir)
	log.Printf("Output format: %s", outputFormat)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
