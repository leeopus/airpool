package subscribe

import (
	"fmt"

	"github.com/airpool/airpool/internal/store"
	"gopkg.in/yaml.v3"
)

type Generator struct {
	store    *store.Store
	hy2Pass  string
}

func New(s *store.Store, hy2Password string) *Generator {
	return &Generator{store: s, hy2Pass: hy2Password}
}

func (g *Generator) Generate() ([]byte, error) {
	nodes, err := g.store.ListOnlineNodes()
	if err != nil {
		return nil, fmt.Errorf("list online nodes: %w", err)
	}
	pools, err := g.store.ListPools()
	if err != nil {
		return nil, fmt.Errorf("list pools: %w", err)
	}

	// Build proxies list
	var proxies []map[string]interface{}
	var allNames []string

	// Group nodes by pool
	poolNodes := make(map[string][]store.Node)
	for _, n := range nodes {
		poolNodes[n.Pool] = append(poolNodes[n.Pool], n)
	}

	for _, n := range nodes {
		proxy := map[string]interface{}{
			"name":           n.Name,
			"type":           "hysteria2",
			"server":         n.IP,
			"port":           443,
			"ports":          "20000-40000",
			"password":       g.hy2Pass,
			"sni":            "www.bing.com",
			"skip-cert-verify": true,
		}
		proxies = append(proxies, proxy)
		allNames = append(allNames, n.Name)
	}

	if len(proxies) == 0 {
		proxies = make([]map[string]interface{}, 0)
	}

	// Build proxy-groups
	var proxyGroups []map[string]interface{}

	// Select group: contains all pool groups + auto + DIRECT
	selectProxies := []string{"🌐 Auto"}
	for _, p := range pools {
		if ns := poolNodes[p.Name]; len(ns) > 0 {
			selectProxies = append(selectProxies, poolGroupName(p.Name))
		}
	}
	selectProxies = append(selectProxies, "DIRECT")

	proxyGroups = append(proxyGroups, map[string]interface{}{
		"name":    "🚀 Select",
		"type":    "select",
		"proxies": selectProxies,
	})

	// Auto group: all online nodes, url-test
	autoGroup := map[string]interface{}{
		"name":      "🌐 Auto",
		"type":      "url-test",
		"url":       "https://www.gstatic.com/generate_204",
		"interval":  30,
		"tolerance": 50,
		"timeout":   2000,
		"proxies":   allNames,
	}
	if len(allNames) == 0 {
		autoGroup["proxies"] = []string{"DIRECT"}
	}
	proxyGroups = append(proxyGroups, autoGroup)

	// Per-pool groups
	for _, p := range pools {
		ns := poolNodes[p.Name]
		if len(ns) == 0 {
			continue
		}
		var names []string
		for _, n := range ns {
			names = append(names, n.Name)
		}
		proxyGroups = append(proxyGroups, map[string]interface{}{
			"name":      poolGroupName(p.Name),
			"type":      "url-test",
			"url":       "https://www.gstatic.com/generate_204",
			"interval":  30,
			"tolerance": 50,
			"timeout":   2000,
			"proxies":   names,
		})
	}

	// Build full config
	config := map[string]interface{}{
		"mixed-port":         7890,
		"allow-lan":          false,
		"mode":               "rule",
		"log-level":          "info",
		"external-controller": "127.0.0.1:9090",
		"dns": map[string]interface{}{
			"enable":       true,
			"enhanced-mode": "fake-ip",
			"nameserver":   []string{"https://dns.alidns.com/dns-query", "https://doh.pub/dns-query"},
			"fallback":     []string{"https://dns.google/dns-query", "https://1.1.1.1/dns-query"},
			"fallback-filter": map[string]interface{}{
				"geoip":  true,
				"ipcidr": []string{"240.0.0.0/4"},
			},
		},
		"proxies":      proxies,
		"proxy-groups": proxyGroups,
		"rules": []string{
			"GEOIP,CN,DIRECT",
			"DOMAIN-SUFFIX,cn,DIRECT",
			"DOMAIN-KEYWORD,baidu,DIRECT",
			"DOMAIN-KEYWORD,alibaba,DIRECT",
			"DOMAIN-KEYWORD,tencent,DIRECT",
			"DOMAIN-KEYWORD,taobao,DIRECT",
			"DOMAIN-KEYWORD,jd.com,DIRECT",
			"DOMAIN-KEYWORD,bilibili,DIRECT",
			"DOMAIN-KEYWORD,163.com,DIRECT",
			"DOMAIN-SUFFIX,local,DIRECT",
			"IP-CIDR,192.168.0.0/16,DIRECT",
			"IP-CIDR,10.0.0.0/8,DIRECT",
			"IP-CIDR,172.16.0.0/12,DIRECT",
			"IP-CIDR,127.0.0.0/8,DIRECT",
			"MATCH,🚀 Select",
		},
	}

	return yaml.Marshal(config)
}

func poolGroupName(pool string) string {
	icons := map[string]string{
		"us":      "🇺🇸",
		"jp":      "🇯🇵",
		"hk":      "🇭🇰",
		"sg":      "🇸🇬",
		"tw":      "🇹🇼",
		"kr":      "🇰🇷",
		"uk":      "🇬🇧",
		"de":      "🇩🇪",
		"premium": "💎",
		"standard": "📦",
		"backup":  "🔄",
	}
	if icon, ok := icons[pool]; ok {
		return fmt.Sprintf("%s %s", icon, pool)
	}
	return fmt.Sprintf("📡 %s", pool)
}
