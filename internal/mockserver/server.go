// Package mockserver is a local, in-memory backend for development and
// integration testing (section 30). It implements the HTTP contract
// described in sections 16/17: pairing and usage sync. It is explicitly a
// development tool — never presented as production (section 21/30), and
// it fabricates no real credentials.
package mockserver

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/iameter/collector/internal/idgen"
)

type pairRequest struct {
	PairingCode string `json:"pairing_code"`
	Device      struct {
		Name             string `json:"name"`
		OS               string `json:"os"`
		Arch             string `json:"arch"`
		CollectorVersion string `json:"collector_version"`
	} `json:"device"`
}

type pairResponse struct {
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
	UserID      string `json:"user_id"`
}

type deviceRecord struct {
	DeviceID string
	Token    string
	UserID   string
}

// Server holds the mock backend's in-memory state.
type Server struct {
	mu             sync.Mutex
	validCodes     map[string]bool         // unused pairing codes accepted
	usedCodes      map[string]bool         // codes already redeemed
	devicesByToken map[string]deviceRecord // token -> device
	seenIdemKeys   map[string]bool         // usage sync Idempotency-Keys already accepted
	usageCount     int
	Logger         *log.Logger
}

// New creates a mock server pre-seeded with the given valid pairing codes
// (callers typically generate and print one for manual `iameter pair`
// testing).
func New(validPairingCodes ...string) *Server {
	s := &Server{
		validCodes:     map[string]bool{},
		usedCodes:      map[string]bool{},
		devicesByToken: map[string]deviceRecord{},
		seenIdemKeys:   map[string]bool{},
	}
	for _, c := range validPairingCodes {
		s.validCodes[c] = true
	}
	return s
}

// pairingCodeAlphabet excludes visually-ambiguous characters (0/O, 1/I/L)
// since pairing codes are meant to be typed by a human.
const pairingCodeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// NewPairingCode generates and registers a fresh valid pairing code, e.g.
// "CM-7X4P2Q9K", for interactive testing.
func (s *Server) NewPairingCode() string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	code := make([]byte, len(buf))
	for i, b := range buf {
		code[i] = pairingCodeAlphabet[int(b)%len(pairingCodeAlphabet)]
	}
	full := "CM-" + string(code)

	s.mu.Lock()
	s.validCodes[full] = true
	s.mu.Unlock()
	return full
}

// Handler returns the http.Handler implementing the mock backend.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/devices/pair", s.handlePair)
	mux.HandleFunc("/v1/collector/usage", s.handleUsage)
	mux.HandleFunc("/v1/devices/heartbeat", s.handleHeartbeat)
	return mux
}

func (s *Server) log(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req pairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.PairingCode == "" || len(req.PairingCode) > 32 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pairing code format"})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.usedCodes[req.PairingCode] {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "pairing code already used"})
		return
	}
	if !s.validCodes[req.PairingCode] {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pairing code not found or expired"})
		return
	}

	deviceID, _ := idgen.New("dev")
	token, _ := idgen.New("iam_device")
	userID, _ := idgen.New("usr")

	delete(s.validCodes, req.PairingCode)
	s.usedCodes[req.PairingCode] = true
	s.devicesByToken[token] = deviceRecord{DeviceID: deviceID, Token: token, UserID: userID}

	s.log("paired device_id=%s os=%s arch=%s", deviceID, req.Device.OS, req.Device.Arch)
	writeJSON(w, http.StatusOK, pairResponse{DeviceID: deviceID, DeviceToken: token, UserID: userID})
}

func (s *Server) authenticate(r *http.Request) (deviceRecord, bool) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		return deviceRecord{}, false
	}
	token := auth[len(prefix):]
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devicesByToken[token]
	return rec, ok
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.authenticate(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing device token"})
		return
	}

	if _, err := decodeJSONBody(w, r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	idemKey := r.Header.Get("Idempotency-Key")
	s.mu.Lock()
	defer s.mu.Unlock()
	if idemKey != "" && s.seenIdemKeys[idemKey] {
		writeJSON(w, http.StatusConflict, map[string]string{"status": "already recorded"})
		return
	}
	if idemKey != "" {
		s.seenIdemKeys[idemKey] = true
	}
	s.usageCount++
	writeJSON(w, http.StatusCreated, map[string]string{"status": "accepted"})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.authenticate(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing device token"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	var body map[string]any
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body)
	return body, err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// UsageCount returns how many usage snapshots have been accepted, for test
// assertions.
func (s *Server) UsageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usageCount
}
