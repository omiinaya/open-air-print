# AirPrint Server in Go - Implementation Plan

---

## Phase 1: Setup & Architecture (Day 1)

### Step 1: Project Structure
```
airprint-server/
├── go.mod
├── main.go              # CLI entry point
├── internal/
│   ├── mdns/           # mDNS advertising
│   │   ├── advertiser.go
│   │   └── resolver.go
│   ├── ipp/            # IPP server
│   │   ├── server.go
│   │   ├── handler.go
│   │   ├── attributes.go
│   │   └── operations.go
│   ├── printer/        # Windows printer backend
│   │   ├── windows.go
│   │   └── job.go
│   ├── config/         # Configuration
│   │   └── config.go
│   └── log/
├── pkg/
│   └── ippproto/       # IPP protocol definitions
└── assets/
    └── icons/          # printer icons for iOS
```

### Step 2: Initialize Go Module
```bash
go mod init airprint-server
go get github.com/grandcat/zeroconf
```

### Step 3: Define IPP Protocol
Create `pkg/ippproto/ipp.go` with:
- IPP message format (RFC 8010)
- Attribute types (integer, boolean, string, etc.)
- Operations: Print-Job (0x0002), Get-Printer-Attributes (0x000B), etc.
- Encode/decode functions

---

## Phase 2: mDNS Advertising (Day 1-2)

### Step 4: Implement Advertiser (`internal/mdns/advertiser.go`)
Use `github.com/grandcat/zeroconf`:
```go
type Advertiser struct {
    server *zeroconf.Server
}

func (a *Advertiser) Start(printerName string, port int, txTRecords map[string]string) error {
    // Register _ipp._tcp and _printer._tcp services
    // TXT records must include:
    //   - "txtvers=1"
    //   - "qtotal=1" (total printers)
    //   - "rp=printers/PrinterName" (resource path)
    //   - "ty=Printer Name"
    //   - "adminurl=http://host:port"
}
```

Required TXT records (AirPrint spec):
- `product=(HP LaserJet, etc.)`
- `priority=0`
- `printertype=0x801002` (AirPrint)
- `printerstate=3` (idle)
- `pdl=application/pdf,image/jpeg,image/png`
- `color=4` (color support)
- `duplex=1` (duplex support)
- `copies=1`

---

## Phase 3: IPP Server (Day 2-4)

### Step 5: IPP Server (`internal/ipp/server.go`)
```go
type Server struct {
    addr    string
    port    int
    handler *Handler
}

func (s *Server) Start() error {
    ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.addr, s.port))
    // Handle connections concurrently
    // Parse IPP request
    // Pass to handler
}
```

### Step 6: IPP Handler (`internal/ipp/handler.go`)
Implement operations:
- **Get-Printer-Attributes** (0x000B): Return printer capabilities
- **Print-Job** (0x0002): Accept document, queue for printing
- **Get-Job-Attributes** (0x0009): Query job status
- **Cancel-Job** (0x0008): Cancel pending job

Printer attributes to return:
- `printer-name`, `printer-location`, `printer-info`
- `printer-state` (3=idle, 4=processing, 5=stopped)
- `printer-state-reasons`
- `queued-job-count`
- `document-format-supported`: `["application/pdf", "image/jpeg"]`
- `color-supported`: `true`
- `copies-supported`: `{1, 2, 3...}`

### Step 7: Job Queue (`internal/ipp/job.go`)
```go
type JobManager struct {
    jobs     map[int]*Job
    mu       sync.RWMutex
    nextID   int
    printer  *windows.Printer
}

type Job struct {
    ID        int
    Format    string  // mime type
    Data      []byte
    UserName  string
    Title     string
    Status    string  // pending, processing, completed, canceled
    CreatedAt time.Time
}
```

---

## Phase 4: Windows Printer Backend (Day 4-5)

### Step 8: Windows Printer Interface (`internal/printer/windows.go`)
Use `github.com/alexbrainman/printer`:
```go
type Printer struct {
    name string
}

func (p *Printer) Print(data []byte, format string) error {
    // Convert data to EMF or send as RAW if printer supports it
    // Call printer.Open(), printer.StartDoc(), printer.Write(), printer.EndDoc()
}

func (p *Printer) GetStatus() (string, error) {
    // Query printer status via Windows API
}
```

**Note:** Windows printers expect EMF (Enhanced Metafile) or RAW (PCL/PS). We need to convert PDF to EMF. Use `github.com/raszia/gotik` for PDF-to-EMF or call Ghostscript.

Alternative: Use CUPS on Windows (install CUPS, use its API) – easier but adds dependency.

---

## Phase 5: Print Data Handling (Day 5)

### Step 9: Format Support
- **PDF**: Extract pages, render to EMF/bitmap (use `github.com/raszia/gotik` or external Ghostscript)
- **JPEG/PNG**: Direct send to printer if printer supports raster
- **AirPrint requirement**: Must support PDF (mandatory)

If using Ghostscript:
```go
func convertPDFtoEMF(pdfData []byte) ([]byte, error) {
    // Call gswin64c with arguments to output EMF
    cmd := exec.Command("gswin64c", "-sDEVICE=emf", "-sOutputFile=-", "-")
    cmd.Stdin = bytes.NewReader(pdfData)
    return cmd.Output()
}
```

---

## Phase 6: Configuration & CLI (Day 6)

### Step 10: Configuration (`internal/config/config.go`)
```go
type Config struct {
    PrinterName   string        `yaml:"printer_name"`   // defaults to system default printer
    Interface     string        `yaml:"interface"`     // "0.0.0.0" or specific IP
    Port          int           `yaml:"port"`          // IPP port (631)
   -mdnsPort     int           `yaml:"mdns_port"`     // 5353
    LogLevel      string        `yaml:"log_level"`
    WebUI         bool          `yaml:"web_ui"`        // optional management UI
    WebPort       int           `yaml:"web_port"`
}
```

### Step 11: Main (`main.go`)
```go
func main() {
    cfg := loadConfig()
    log := setupLogging(cfg)

    // 1. Get printer
    printer := windows.NewDefaultPrinter()

    // 2. Start IPP server
    ippSrv := ipp.NewServer(cfg.Interface, cfg.Port, printer)
    go ippSrv.Start()

    // 3. Start mDNS advertiser
    mdns := mdns.NewAdvertiser()
    mdns.Start(cfg.PrinterName, cfg.Port, buildTXTRecords(cfg))

    // 4. Optional Web UI
    if cfg.WebUI {
        startWebUI(cfg.WebPort, ippSrv)
    }

    waitForSignal()
}
```

---

## Phase 7: Testing (Day 7)

### Step 12: Unit Tests
- Test IPP request/response encoding
- Test job queue operations
- Test config parsing

### Step 13: Integration Testing
1. Run server on Windows machine
2. Connect iPhone/iPad to same WiFi
3. Open Safari, print a webpage → should see printer
4. Print PDF, image → verify output
5. Test multiple jobs, cancel job

---

## Phase 8: Polish & Release (Day 8)

### Step 14: Error Handling & Logging
- Structured logging (`slog` or `zap`)
- Graceful shutdown (handle SIGINT)
- Recover from panics

### Step 15: Build & Package
```bash
# Build for Windows
GOOS=windows GOARCH=amd64 go build -o airprint-server.exe

# Optional: embed web UI
// Use embed package to include static files
```

---

## Potential Pitfalls & Solutions

| Issue | Solution |
|-------|----------|
| Windows printer driver compatibility | Use generic "Microsoft Print to PDF" as fallback, or require PostScript/PCL printer |
| PDF conversion complexity | Use Ghostscript external dependency (most reliable) |
| mDNS not working on some networks | Add option to broadcast via fixed IP |
| Memory leaks in job queue | Use context cancellation, cleanup completed jobs |
| Multiple iOS devices print simultaneously | Mutex job queue, process sequentially or parallel if printer supports |

---

## Estimated Timeline: **8 days** (working 4-6 hours/day)

- Day 1: Project setup, mDNS
- Day 2-3: IPP server core
- Day 4: Windows printer backend
- Day 5: PDF handling
- Day 6: CLI/config
- Day 7: Testing/debug
- Day 8: Polish, docs, build

---

**Start building?** I can generate the initial code structure and core files.
