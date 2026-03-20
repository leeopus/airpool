package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Pool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
}

type Node struct {
	Name         string `json:"name"`
	Pool         string `json:"pool"`
	IP           string `json:"ip"`
	Status       string `json:"status"`
	FailCount    int    `json:"fail_count"`
	LastCheckAt  *int64 `json:"last_check_at"`
	LastOnlineAt *int64 `json:"last_online_at"`
	CreatedAt    int64  `json:"created_at"`
}

type Event struct {
	ID        int64  `json:"id"`
	NodeName  string `json:"node_name"`
	EventType string `json:"event_type"`
	Detail    string `json:"detail"`
	CreatedAt int64  `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Pool operations

func (s *Store) CreatePool(name, description string) error {
	_, err := s.db.Exec("INSERT INTO pools (name, description) VALUES (?, ?)", name, description)
	return err
}

func (s *Store) GetPool(name string) (*Pool, error) {
	p := &Pool{}
	err := s.db.QueryRow("SELECT name, description, created_at FROM pools WHERE name = ?", name).
		Scan(&p.Name, &p.Description, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Store) ListPools() ([]Pool, error) {
	rows, err := s.db.Query("SELECT name, description, created_at FROM pools ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pools []Pool
	for rows.Next() {
		var p Pool
		if err := rows.Scan(&p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		pools = append(pools, p)
	}
	return pools, rows.Err()
}

func (s *Store) UpdatePool(name, description string) error {
	res, err := s.db.Exec("UPDATE pools SET description = ? WHERE name = ?", description, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("pool %q not found", name)
	}
	return nil
}

func (s *Store) DeletePool(name string) error {
	// Check for nodes in pool
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE pool = ?", name).Scan(&count)
	if count > 0 {
		return fmt.Errorf("pool %q has %d nodes, remove them first", name, count)
	}
	_, err := s.db.Exec("DELETE FROM pools WHERE name = ?", name)
	return err
}

// Node operations

func (s *Store) CreateNode(name, pool, ip string) error {
	_, err := s.db.Exec("INSERT INTO nodes (name, pool, ip) VALUES (?, ?, ?)", name, pool, ip)
	return err
}

func (s *Store) GetNodeByIP(ip string) (*Node, error) {
	n := &Node{}
	err := s.db.QueryRow(
		"SELECT name, pool, ip, status, fail_count, last_check_at, last_online_at, created_at FROM nodes WHERE ip = ?", ip,
	).Scan(&n.Name, &n.Pool, &n.IP, &n.Status, &n.FailCount, &n.LastCheckAt, &n.LastOnlineAt, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return n, err
}

func (s *Store) GetNode(name string) (*Node, error) {
	n := &Node{}
	err := s.db.QueryRow(
		"SELECT name, pool, ip, status, fail_count, last_check_at, last_online_at, created_at FROM nodes WHERE name = ?", name,
	).Scan(&n.Name, &n.Pool, &n.IP, &n.Status, &n.FailCount, &n.LastCheckAt, &n.LastOnlineAt, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return n, err
}

func (s *Store) ListNodes() ([]Node, error) {
	rows, err := s.db.Query("SELECT name, pool, ip, status, fail_count, last_check_at, last_online_at, created_at FROM nodes ORDER BY pool, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.Name, &n.Pool, &n.IP, &n.Status, &n.FailCount, &n.LastCheckAt, &n.LastOnlineAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *Store) ListOnlineNodes() ([]Node, error) {
	rows, err := s.db.Query("SELECT name, pool, ip, status, fail_count, last_check_at, last_online_at, created_at FROM nodes WHERE status = 'online' ORDER BY pool, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.Name, &n.Pool, &n.IP, &n.Status, &n.FailCount, &n.LastCheckAt, &n.LastOnlineAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *Store) DeleteNode(name string) error {
	_, err := s.db.Exec("DELETE FROM nodes WHERE name = ?", name)
	return err
}

func (s *Store) UpdateNodeStatus(name, status string) error {
	now := time.Now().Unix()
	var query string
	switch status {
	case "online":
		query = "UPDATE nodes SET status = ?, fail_count = 0, last_check_at = ?, last_online_at = ? WHERE name = ?"
		_, err := s.db.Exec(query, status, now, now, name)
		return err
	case "offline":
		query = "UPDATE nodes SET status = ?, last_check_at = ? WHERE name = ?"
		_, err := s.db.Exec(query, status, now, name)
		return err
	default:
		query = "UPDATE nodes SET status = ? WHERE name = ?"
		_, err := s.db.Exec(query, status, name)
		return err
	}
}

func (s *Store) UpdateNodePool(name, pool string) error {
	res, err := s.db.Exec("UPDATE nodes SET pool = ? WHERE name = ?", pool, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("node %q not found", name)
	}
	return nil
}

func (s *Store) IncrementFailCount(name string) (int, error) {
	now := time.Now().Unix()
	_, err := s.db.Exec("UPDATE nodes SET fail_count = fail_count + 1, last_check_at = ? WHERE name = ?", now, name)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.db.QueryRow("SELECT fail_count FROM nodes WHERE name = ?", name).Scan(&count)
	return count, err
}

func (s *Store) MarkOnline(name string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec("UPDATE nodes SET status = 'online', fail_count = 0, last_check_at = ?, last_online_at = ? WHERE name = ?", now, now, name)
	return err
}

// Event operations

func (s *Store) AddEvent(nodeName, eventType, detail string) error {
	_, err := s.db.Exec("INSERT INTO events (node_name, event_type, detail) VALUES (?, ?, ?)", nodeName, eventType, detail)
	return err
}

func (s *Store) ListEvents(limit int) ([]Event, error) {
	rows, err := s.db.Query("SELECT id, node_name, event_type, detail, created_at FROM events ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.NodeName, &e.EventType, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
