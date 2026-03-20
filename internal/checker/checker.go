package checker

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/airpool/airpool/internal/alert"
	"github.com/airpool/airpool/internal/store"
)

type Checker struct {
	store            *store.Store
	alerter          *alert.Alerter
	interval         time.Duration
	timeout          time.Duration
	offlineThreshold int
	stop             chan struct{}
}

func New(s *store.Store, a *alert.Alerter, intervalSec, timeoutSec, threshold int) *Checker {
	return &Checker{
		store:            s,
		alerter:          a,
		interval:         time.Duration(intervalSec) * time.Second,
		timeout:          time.Duration(timeoutSec) * time.Second,
		offlineThreshold: threshold,
		stop:             make(chan struct{}),
	}
}

func (c *Checker) Start() {
	log.Printf("[checker] started, interval=%s, timeout=%s, threshold=%d", c.interval, c.timeout, c.offlineThreshold)
	go c.loop()
}

func (c *Checker) Stop() {
	close(c.stop)
}

func (c *Checker) loop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Run immediately on start
	c.checkAll()

	for {
		select {
		case <-ticker.C:
			c.checkAll()
		case <-c.stop:
			return
		}
	}
}

func (c *Checker) checkAll() {
	nodes, err := c.store.ListNodes()
	if err != nil {
		log.Printf("[checker] list nodes error: %v", err)
		return
	}
	if len(nodes) == 0 {
		return
	}

	sem := make(chan struct{}, 20) // concurrency limit
	var wg sync.WaitGroup

	for _, node := range nodes {
		if node.Status == "disabled" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(n store.Node) {
			defer wg.Done()
			defer func() { <-sem }()
			c.checkNode(n)
		}(node)
	}
	wg.Wait()
}

func (c *Checker) checkNode(node store.Node) {
	addr := fmt.Sprintf("%s:443", node.IP)
	conn, err := net.DialTimeout("udp", addr, c.timeout)
	if err != nil {
		c.handleFailure(node)
		return
	}
	defer conn.Close()

	// Send a minimal QUIC Initial packet (version 1, any payload).
	// A live QUIC server will respond; a dead port will trigger an
	// ICMP port-unreachable which surfaces as a read error.
	conn.SetDeadline(time.Now().Add(c.timeout))

	// QUIC long header: Initial packet (type 0xc0), version 1
	pkt := []byte{
		0xc0,                   // long header, Initial
		0x00, 0x00, 0x00, 0x01, // QUIC version 1
		0x08,                                                       // DCID length = 8
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,            // DCID
		0x00, // SCID length = 0
		0x00, // token length = 0
		0x00, 0x01, // payload length = 1
		0x00, // minimal payload
	}

	if _, err := conn.Write(pkt); err != nil {
		c.handleFailure(node)
		return
	}

	buf := make([]byte, 1500)
	_, err = conn.Read(buf)
	if err != nil {
		// Read error (timeout or ICMP unreachable) = port not responding
		c.handleFailure(node)
		return
	}
	c.handleSuccess(node)
}

func (c *Checker) handleSuccess(node store.Node) {
	prevStatus := node.Status
	if err := c.store.MarkOnline(node.Name); err != nil {
		log.Printf("[checker] mark online %s error: %v", node.Name, err)
		return
	}
	if prevStatus != "online" {
		log.Printf("[checker] %s: %s → online", node.Name, prevStatus)
		c.store.AddEvent(node.Name, "online", fmt.Sprintf("%s → online", prevStatus))
		if prevStatus == "offline" && c.alerter.Enabled() {
			c.alerter.NodeOnline(node.Name, node.IP, node.Pool)
		}
	}
}

func (c *Checker) handleFailure(node store.Node) {
	failCount, err := c.store.IncrementFailCount(node.Name)
	if err != nil {
		log.Printf("[checker] increment fail %s error: %v", node.Name, err)
		return
	}

	if node.Status != "offline" && failCount >= c.offlineThreshold {
		if err := c.store.UpdateNodeStatus(node.Name, "offline"); err != nil {
			log.Printf("[checker] mark offline %s error: %v", node.Name, err)
			return
		}
		log.Printf("[checker] %s: %s → offline (fail_count=%d)", node.Name, node.Status, failCount)
		c.store.AddEvent(node.Name, "offline", fmt.Sprintf("health check failed %d times", failCount))
		if c.alerter.Enabled() {
			c.alerter.NodeOffline(node.Name, node.IP, node.Pool)
		}
	}
}
