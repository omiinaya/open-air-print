package mdns

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"
)

// Advertiser handles mDNS/Bonjour advertising of AirPrint services
type Advertiser struct {
	server     *zeroconf.Server
	printerName string
	port       int
	txtRecords map[string]string
	mu         sync.RWMutex
}

// NewAdvertiser creates a new mDNS advertiser
func NewAdvertiser() *Advertiser {
	return &Advertiser{
		txtRecords: make(map[string]string),
	}
}

// Start begins advertising the AirPrint service on the network
func (a *Advertiser) Start(printerName string, port int, additionalTXT map[string]string) error {
	a.mu.Lock()
	a.printerName = printerName
	a.port = port

	// Build TXT records with AirPrint-required attributes
	txt := map[string]string{
		"txtvers":            "1",
		"qtotal":             "1",
		"rp":                 fmt.Sprintf("printers/%s", url.PathEscape(printerName)),
		"ty":                 printerName,
		"adminurl":           fmt.Sprintf("http://%s:%d", getLocalIP(), port),
		"product":            additionalTXT["product"],
		"priority":           "0",
		"printertype":        "0x801002", // AirPrint
		"printerstate":       "3",        // idle
		"pdl":                "application/pdf,image/jpeg,image/png",
		"color":              "4",        // color support
		"duplex":             "1",        // duplex support
		"copies":             "1",
		"note":               additionalTXT["note"],
	}

	// Merge any additional TXT records
	for k, v := range additionalTXT {
		if _, exists := txt[k]; !exists {
			txt[k] = v
		}
	}

	a.txtRecords = txt
	a.mu.Unlock()

	// Convert map to []string "key=value" format
	var txtRecords []string
	for k, v := range txt {
		txtRecords = append(txtRecords, k+"="+v)
	}

	// Register both _ipp._tcp and _printer._tcp services
	// AirPrint clients look for both
	server, err := zeroconf.Register(
		printerName+"-ipp",  // instance name
		"_ipp._tcp",         // service type
		"local.",            // domain
		port,
		txtRecords,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register _ipp._tcp service: %w", err)
	}

	// Also register _printer._tcp for broader compatibility
	_, err = zeroconf.Register(
		printerName,
		"_printer._tcp",
		"local.",
		port,
		txtRecords,
		nil,
	)
	if err != nil {
		// Not a fatal error, but log it
		log.Printf("Warning: failed to register _printer._tcp service: %v", err)
	}

	a.server = server
	log.Printf("mDNS advertiser started: advertising '%s' on port %d", printerName, port)
	return nil
}

// Stop shuts down the mDNS advertiser
func (a *Advertiser) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.server != nil {
		a.server.Shutdown()
		a.server = nil
		log.Println("mDNS advertiser stopped")
	}
}

// UpdateTXT updates TXT records (e.g., printer state)
func (a *Advertiser) UpdateTXT(updates map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for k, v := range updates {
		a.txtRecords[k] = v
	}

	// Note: zeroconf doesn't provide a direct way to update TXT records
	// In production, you'd need to re-register or use a different library
	// For now, we'll just log that we'd want to update
	log.Printf("TXT records updated (note: not automatically re-advertised without restart)")
}

// Helper: get local IP address for adminurl
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "localhost"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// Helper: escape special characters in strings for DNS
func escapeString(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			fallthrough
		case r >= 'a' && r <= 'z':
			fallthrough
		case r >= '0' && r <= '9':
			fallthrough
		case r == '-' || r == '.' || r == '_':
			result.WriteRune(r)
		default:
			// Space or special char, use \xhh
			fmt.Fprintf(&result, "\\%02x", r)
		}
	}
	return result.String()
}
