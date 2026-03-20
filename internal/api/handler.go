package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/airpool/airpool/web"

	"github.com/airpool/airpool/internal/config"
	"github.com/airpool/airpool/internal/store"
	"github.com/airpool/airpool/internal/subscribe"
)

type Handler struct {
	cfg        *config.Config
	cfgPath    string
	store      *store.Store
	generator  *subscribe.Generator
}

func New(cfg *config.Config, cfgPath string, s *store.Store, gen *subscribe.Generator) *Handler {
	return &Handler{cfg: cfg, cfgPath: cfgPath, store: s, generator: gen}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/pools", h.handlePools)
	mux.HandleFunc("/api/pools/", h.handlePoolByName)
	mux.HandleFunc("/api/nodes", h.handleNodes)
	mux.HandleFunc("/api/nodes/", h.handleNodeByName)
	mux.HandleFunc("/api/subscribe", h.handleSubscribe)
	mux.HandleFunc("/api/config", h.handleConfig)
	mux.HandleFunc("/api/events", h.handleEvents)
	mux.HandleFunc("/api/tokens/regenerate", h.handleRegenerateToken)
	mux.HandleFunc("/install.sh", h.handleInstallScript)
	webFS, _ := fs.Sub(web.FS, ".")
	mux.Handle("/", http.FileServer(http.FS(webFS)))
}

// --- Pools ---

func (h *Handler) handlePools(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.requireAPIToken(w, r, h.listPools)
	case http.MethodPost:
		h.requireAPIToken(w, r, h.createPool)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handlePoolByName(w http.ResponseWriter, r *http.Request) {
	h.requireAPIToken(w, r, func(w http.ResponseWriter, r *http.Request) {
		// /api/pools/{name}
		name := strings.TrimPrefix(r.URL.Path, "/api/pools/")
		if name == "" {
			http.Error(w, "pool name required", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.getPool(w, r, name)
		case http.MethodPut:
			h.updatePool(w, r, name)
		case http.MethodDelete:
			h.deletePool(w, r, name)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func (h *Handler) listPools(w http.ResponseWriter, r *http.Request) {
	pools, err := h.store.ListPools()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, pools)
}

func (h *Handler) createPool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if req.Name == "auto" {
		jsonError(w, "cannot create reserved pool 'auto'", http.StatusBadRequest)
		return
	}
	if err := h.store.CreatePool(req.Name, req.Description); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	log.Printf("[api] pool created: %s", req.Name)
	jsonOK(w, map[string]string{"status": "ok", "name": req.Name})
}

func (h *Handler) getPool(w http.ResponseWriter, r *http.Request, name string) {
	pool, err := h.store.GetPool(name)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pool == nil {
		jsonError(w, "pool not found", http.StatusNotFound)
		return
	}
	jsonOK(w, pool)
}

func (h *Handler) updatePool(w http.ResponseWriter, r *http.Request, name string) {
	var req struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.store.UpdatePool(name, req.Description); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *Handler) deletePool(w http.ResponseWriter, r *http.Request, name string) {
	if name == "auto" {
		jsonError(w, "cannot delete reserved pool 'auto'", http.StatusBadRequest)
		return
	}
	if err := h.store.DeletePool(name); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	log.Printf("[api] pool deleted: %s", name)
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Nodes ---

func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.requireAPIToken(w, r, h.listNodes)
	case http.MethodPost:
		h.requireAPIToken(w, r, h.createNode)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleNodeByName(w http.ResponseWriter, r *http.Request) {
	h.requireAPIToken(w, r, func(w http.ResponseWriter, r *http.Request) {
		// /api/nodes/{name} or /api/nodes/{name}/{action}
		path := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
		parts := strings.SplitN(path, "/", 2)
		name := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		if name == "" {
			http.Error(w, "node name required", http.StatusBadRequest)
			return
		}

		switch {
		case action == "" && r.Method == http.MethodGet:
			h.getNode(w, r, name)
		case action == "" && r.Method == http.MethodDelete:
			h.deleteNode(w, r, name)
		case action == "online" && r.Method == http.MethodPut:
			h.setNodeStatus(w, r, name, "online")
		case action == "offline" && r.Method == http.MethodPut:
			h.setNodeStatus(w, r, name, "offline")
		case action == "pool" && r.Method == http.MethodPut:
			h.moveNodePool(w, r, name)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
}

func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.store.ListNodes()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, nodes)
}

func (h *Handler) createNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Pool string `json:"pool"`
		IP   string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Pool == "" || req.IP == "" {
		jsonError(w, "pool and ip required", http.StatusBadRequest)
		return
	}
	// Verify pool exists
	pool, err := h.store.GetPool(req.Pool)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pool == nil {
		jsonError(w, fmt.Sprintf("pool %q not found, create it first", req.Pool), http.StatusBadRequest)
		return
	}
	// Auto-generate name if empty
	if req.Name == "" {
		req.Name = fmt.Sprintf("%s-%s", req.Pool, randomHex(4))
	}
	if err := h.store.CreateNode(req.Name, req.Pool, req.IP); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	log.Printf("[api] node registered: %s (pool=%s, ip=%s)", req.Name, req.Pool, req.IP)
	h.store.AddEvent(req.Name, "registered", fmt.Sprintf("pool=%s, ip=%s", req.Pool, req.IP))
	jsonOK(w, map[string]string{"status": "ok", "name": req.Name})
}

func (h *Handler) getNode(w http.ResponseWriter, r *http.Request, name string) {
	node, err := h.store.GetNode(name)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if node == nil {
		jsonError(w, "node not found", http.StatusNotFound)
		return
	}
	jsonOK(w, node)
}

func (h *Handler) deleteNode(w http.ResponseWriter, r *http.Request, name string) {
	if err := h.store.DeleteNode(name); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[api] node deleted: %s", name)
	h.store.AddEvent(name, "deleted", "")
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *Handler) setNodeStatus(w http.ResponseWriter, r *http.Request, name, status string) {
	node, err := h.store.GetNode(name)
	if err != nil || node == nil {
		jsonError(w, "node not found", http.StatusNotFound)
		return
	}
	if err := h.store.UpdateNodeStatus(name, status); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("[api] node %s manually set to %s", name, status)
	h.store.AddEvent(name, status, "manual operation")
	jsonOK(w, map[string]string{"status": "ok"})
}

func (h *Handler) moveNodePool(w http.ResponseWriter, r *http.Request, name string) {
	var req struct {
		Pool string `json:"pool"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Pool == "" {
		jsonError(w, "pool required", http.StatusBadRequest)
		return
	}
	// Verify pool exists
	pool, err := h.store.GetPool(req.Pool)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pool == nil {
		jsonError(w, fmt.Sprintf("pool %q not found", req.Pool), http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateNodePool(name, req.Pool); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	log.Printf("[api] node %s moved to pool %s", name, req.Pool)
	h.store.AddEvent(name, "pool_changed", fmt.Sprintf("moved to %s", req.Pool))
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- Subscribe ---

func (h *Handler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := r.URL.Query().Get("token")
	if token != h.cfg.SubscribeToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	data, err := h.generator.Generate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"airpool.yaml\"")
	w.Header().Set("Subscription-Userinfo", "upload=0; download=0; total=0; expire=0")
	w.Write(data)
}

// --- Config (get hy2 password for install script) ---

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.requireAPIToken(w, r, func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]string{
			"hysteria2_password": h.cfg.Hysteria2Password,
			"subscribe_token":   h.cfg.SubscribeToken,
		})
	})
}

// --- Token Regeneration ---

func (h *Handler) handleRegenerateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.requireAPIToken(w, r, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target string `json:"target"` // "api_token", "subscribe_token", or "hysteria2_password"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid json", http.StatusBadRequest)
			return
		}

		var newVal string
		switch req.Target {
		case "api_token":
			newVal = config.GenerateToken("ak_")
			h.cfg.APIToken = newVal
		case "subscribe_token":
			newVal = config.GenerateToken("st_")
			h.cfg.SubscribeToken = newVal
		case "hysteria2_password":
			newVal = config.GenerateToken("hp_")
			h.cfg.Hysteria2Password = newVal
		default:
			jsonError(w, "target must be one of: api_token, subscribe_token, hysteria2_password", http.StatusBadRequest)
			return
		}

		if err := config.Save(h.cfgPath, h.cfg); err != nil {
			jsonError(w, "save config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[api] token regenerated: %s", req.Target)
		resp := map[string]string{"status": "ok", "target": req.Target, "new_value": newVal}
		if req.Target == "api_token" {
			resp["warning"] = "API Token changed. Use the new token for all future requests."
		}
		if req.Target == "hysteria2_password" {
			resp["warning"] = "Hysteria2 password changed. All nodes must be redeployed with the new password."
		}
		jsonOK(w, resp)
	})
}

// --- Events ---

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.requireAPIToken(w, r, func(w http.ResponseWriter, r *http.Request) {
		events, err := h.store.ListEvents(100)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, events)
	})
}

// --- Install Script ---

func (h *Handler) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(installScript))
}

// --- Auth middleware ---

func (h *Handler) requireAPIToken(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token != h.cfg.APIToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	next(w, r)
}

// --- Helpers ---

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

const installScript = `#!/usr/bin/env bash
set -euo pipefail

# AirPool Node Installer
# Usage: curl -skL https://CONFIG_CENTER/install.sh | bash -s -- --server IP:PORT --token TOKEN --pool POOL [--name NAME]

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[AirPool]${NC} $*"; }
warn() { echo -e "${YELLOW}[AirPool]${NC} $*"; }
err()  { echo -e "${RED}[AirPool]${NC} $*"; exit 1; }

SERVER=""
TOKEN=""
POOL=""
NAME=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --server) SERVER="$2"; shift 2 ;;
        --token)  TOKEN="$2";  shift 2 ;;
        --pool)   POOL="$2";   shift 2 ;;
        --name)   NAME="$2";   shift 2 ;;
        *) err "Unknown option: $1" ;;
    esac
done

[[ -z "$SERVER" ]] && err "--server is required"
[[ -z "$TOKEN"  ]] && err "--token is required"
[[ -z "$POOL"   ]] && err "--pool is required"

# Must be root
[[ $EUID -ne 0 ]] && err "Please run as root"

NODE_IP=$(curl -s4 ifconfig.me || curl -s4 icanhazip.com)
[[ -z "$NODE_IP" ]] && err "Cannot detect public IP"

log "Node IP: $NODE_IP"

# 1. Get Hysteria2 password from config center
log "Fetching Hysteria2 password from config center..."
HY2_PASS=$(curl -skf "https://${SERVER}/api/config?token=${TOKEN}" | grep -o '"hysteria2_password":"[^"]*"' | cut -d'"' -f4)
[[ -z "$HY2_PASS" ]] && err "Failed to get Hysteria2 password from config center"

# 2. Install Hysteria2
log "Installing Hysteria2..."
if command -v hysteria &>/dev/null; then
    warn "Hysteria2 already installed, skipping..."
else
    bash <(curl -fsSL https://get.hy2.sh/)
fi

# 3. Generate self-signed certificate
log "Generating self-signed certificate..."
CERT_DIR="/etc/hysteria"
mkdir -p "$CERT_DIR"
if [[ ! -f "$CERT_DIR/server.crt" ]]; then
    openssl req -x509 -nodes -newkey ec:<(openssl ecparam -name prime256v1) \
        -keyout "$CERT_DIR/server.key" -out "$CERT_DIR/server.crt" \
        -subj "/CN=www.bing.com" -days 3650 2>/dev/null
fi

# 4. Write Hysteria2 config
log "Writing Hysteria2 config..."
cat > /etc/hysteria/config.yaml << HYSTERIA_EOF
listen: :443

tls:
  cert: /etc/hysteria/server.crt
  key: /etc/hysteria/server.key

auth:
  type: password
  password: ${HY2_PASS}

masquerade:
  type: proxy
  proxy:
    url: https://www.bing.com
    rewriteHost: true
HYSTERIA_EOF

# 5. Setup port hopping (iptables)
log "Configuring port hopping (20000-40000 -> 443)..."
# Clean old rules if any
iptables -t nat -D PREROUTING -p udp --dport 20000:40000 -j REDIRECT --to-ports 443 2>/dev/null || true
iptables -t nat -A PREROUTING -p udp --dport 20000:40000 -j REDIRECT --to-ports 443

# Persist iptables rules
if command -v netfilter-persistent &>/dev/null; then
    netfilter-persistent save
elif command -v iptables-save &>/dev/null; then
    iptables-save > /etc/iptables.rules
    # Ensure rules load on boot
    if [[ ! -f /etc/network/if-pre-up.d/iptables ]]; then
        cat > /etc/network/if-pre-up.d/iptables << 'IPTEOF'
#!/bin/bash
iptables-restore < /etc/iptables.rules
IPTEOF
        chmod +x /etc/network/if-pre-up.d/iptables
    fi
fi

# 6. Create systemd service and start
log "Starting Hysteria2..."
cat > /etc/systemd/system/hysteria-server.service << 'SVCEOF'
[Unit]
Description=Hysteria2 Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hysteria server -c /etc/hysteria/config.yaml
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SVCEOF

systemctl daemon-reload
systemctl enable hysteria-server
systemctl restart hysteria-server

# Wait a moment for service to start
sleep 2

# Verify service is running
if ! systemctl is-active --quiet hysteria-server; then
    err "Hysteria2 failed to start. Check: journalctl -u hysteria-server"
fi

# 7. Register with config center
log "Registering with config center..."
REGISTER_DATA="{\"pool\":\"${POOL}\",\"ip\":\"${NODE_IP}\""
if [[ -n "$NAME" ]]; then
    REGISTER_DATA="${REGISTER_DATA},\"name\":\"${NAME}\""
fi
REGISTER_DATA="${REGISTER_DATA}}"

RESP=$(curl -skf -X POST "https://${SERVER}/api/nodes" \
    -H "Authorization: ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$REGISTER_DATA") || err "Failed to register with config center"

NODE_NAME=$(echo "$RESP" | grep -o '"name":"[^"]*"' | cut -d'"' -f4)

log "========================================="
log "  Node deployed successfully!"
log "  Name: ${NODE_NAME}"
log "  Pool: ${POOL}"
log "  IP:   ${NODE_IP}"
log "========================================="
`
