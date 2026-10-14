package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type config struct {
	addr   string
	dbHost string
	dbPort string
	dbUser string
	dbPass string
	dbName string
	table  string
}

func loadConfig() config {
	return config{
		addr:   envOr("AUTOMATION_STATE_ADDR", ":10000"),
		dbHost: envOr("AUTOMATION_STATE_DB_HOST", "127.0.0.1"),
		dbPort: envOr("AUTOMATION_STATE_DB_PORT", "3306"),
		dbUser: envOr("AUTOMATION_STATE_DB_USER", "automation_state"),
		dbPass: envOr("AUTOMATION_STATE_DB_PASSWORD", ""),
		dbName: envOr("AUTOMATION_STATE_DB_NAME", "automation_state"),
		table:  envOr("AUTOMATION_STATE_TABLE", "automation_state"),
	}
}

func (c config) dsn() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4",
		c.dbUser, c.dbPass, c.dbHost, c.dbPort, c.dbName)
}

type store interface {
	set(key string, value []byte) error
	all() (map[string]json.RawMessage, error)
}

type stateStore struct {
	db    *sql.DB
	table string
}

func (s *stateStore) ensureTable() error {
	create := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			state_key   VARCHAR(255) NOT NULL PRIMARY KEY,
			state_value LONGTEXT     NOT NULL,
			updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		s.table)
	_, err := s.db.Exec(create)
	return err
}

func (s *stateStore) set(key string, value []byte) error {
	_, err := s.db.Exec(
		fmt.Sprintf("INSERT INTO %s (state_key, state_value) VALUES (?, ?) "+
			"ON DUPLICATE KEY UPDATE state_value = VALUES(state_value)", s.table),
		key, string(value))
	return err
}

func (s *stateStore) all() (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(fmt.Sprintf("SELECT state_key, state_value FROM %s", s.table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	state := make(map[string]json.RawMessage)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		state[k] = json.RawMessage(v)
	}
	return state, rows.Err()
}

type server struct {
	store store
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if !s.applyPost(w, r) {
			return
		}
	}
	s.writeState(w, r)
}

func (s *server) applyPost(w http.ResponseWriter, r *http.Request) bool {
	var body struct {
		StateKey   string          `json:"state_key"`
		StateValue json.RawMessage `json:"state_value"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if body.StateKey == "" {
		http.Error(w, "state_key is required", http.StatusBadRequest)
		return false
	}
	if len(body.StateValue) == 0 {
		http.Error(w, "state_value is required", http.StatusBadRequest)
		return false
	}
	if err := s.store.set(body.StateKey, body.StateValue); err != nil {
		http.Error(w, "failed to store state: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	return true
}

func (s *server) writeState(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.all()
	if err != nil {
		http.Error(w, "failed to read state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "failed to encode state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func main() {
	cfg := loadConfig()

	db, err := sql.Open("mysql", cfg.dsn())
	if err != nil {
		log.Fatalf("invalid DB configuration: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	st := &stateStore{db: db, table: cfg.table}
	if err := waitReady(db, st, 30*time.Second); err != nil {
		log.Fatalf("could not reach MariaDB / ensure table: %v", err)
	}

	srv := &server{store: st}
	http.HandleFunc("/", srv.handle)
	log.Printf("automationstate listening on %s (db %s:%s/%s, table %s)",
		cfg.addr, cfg.dbHost, cfg.dbPort, cfg.dbName, cfg.table)
	if err := http.ListenAndServe(cfg.addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func waitReady(db *sql.DB, st *stateStore, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := db.Ping(); err == nil {
			return st.ensureTable()
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("db unreachable for %s: %w", timeout, lastErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
