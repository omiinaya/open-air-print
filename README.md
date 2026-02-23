# AirPrint Server for Windows

A lightweight, native Windows AirPrint server written in Go. Allows iPhones and iPads to discover and print to any Windows-attached printer via AirPrint/MDNS.

**Status:** Early implementation - Core functionality works. Supports PDF printers natively. Non-PDF printers need Ghostscript conversion (TODO).

---

## Features

- ✅ mDNS/Bonjour advertising (`_ipp._tcp`, `_printer._tcp`)
- ✅ IPP 2.0 server (HTTP on port 631)
- ✅ Windows printer backend via Win32 spooler
- ✅ Job queue with cancelation support
- ✅ Automatic discovery from iPhone/iPad
- ✅ Single binary, no external dependencies (except optional Ghostscript)
- ✅ Configurable via YAML

---

## Requirements

### Hardware/OS
- Windows 10 or 11 (64-bit)
- A printer connected to the Windows machine
- Both iOS device and Windows machine on the same WiFi network

### Software
- **Go 1.21+** (for building from source) - https://go.dev/dl/
  - OR download pre-built binary (release page - coming soon)
- **Optional:** [Ghostscript](https://ghostscript.com/releases/gsdnld.html) if your printer doesn't support PDF directly

---

## Quick Start

### 1. Build from Source

```powershell
# Clone the repository
git clone https://github.com/yourusername/airprint-server
cd airprint-server

# Build for Windows
go build -o airprint-server.exe ./...

# Optional: cross-compile from Linux/macOS
# GOOS=windows GOARCH=amd64 go build -o airprint-server.exe ./...
```

### 2. Configure (Optional)

Default configuration:
- Printer: Windows default printer
- Server port: 631
- mDNS name: "AirPrintServer"
- Published on all network interfaces (0.0.0.0)

Create a config file at `%USERPROFILE%\.airprint-server.yaml`:

```yaml
printer_name: "Office Printer"
printer_model: "HP LaserJet Pro M404dn"
printer_location: "Second Floor"
interface: "0.0.0.0"
port: 631
web_ui: true
web_port: 8080
```

### 3. Run

```powershell
# Run as Administrator (required for printer access)
.\airprint-server.exe
```

On first run, it will:
1. Detect the default Windows printer
2. Start the IPP server on port 631
3. Advertise the printer via mDNS

**Note:** If port 631 is already in use by another service (e.g., Windows built-in IPP), kill that service or change `port` in config.

Keep the terminal window open. The server runs in the foreground.

### 4. Test from iPhone/iPad

1. Connect iPhone/iPad to the same WiFi network as the Windows machine
2. Open a webpage in Safari
3. Tap **Share** → **Print**
4. Your printer should appear as "AirPrintServer" (or your configured name)
5. Select it and print

---

## Configuration

Configuration file location: `%USERPROFILE%\.airprint-server.yaml`

| Option | Default | Description |
|--------|---------|-------------|
| `printer_name` | "AirPrintServer" | Name shown in iOS printer picker |
| `printer_model` | "Generic AirPrint" | Printer model shown in iOS |
| `printer_location` | "Local" | Location text |
| `interface` | "0.0.0.0" | Network interface to bind (use specific IP to restrict) |
| `port` | 631 | IPP server port (must be 631 for AirPrint) |
| `web_ui` | true | Enable simple web status page at `WebPort` |
| `web_port` | 8080 | Web UI port |
| `log_level` | "info" | Logging verbosity: debug, info, warn, error |

After editing the config, restart the server.

---

## Troubleshooting

### Printer doesn't appear on iPhone

**Check:**
- Both devices on same WiFi subnet
- Windows Firewall allows port 631 (TCP) inbound
- mDNS/Bonjour traffic allowed (UDP 5353)
- Printer name is not empty and doesn't contain special characters

**Verify mDNS:** Install [Apple Bonjour Print Services](https://support.apple.com/kb/DL999) or use `dns-sd -B _ipp._tcp local` to browse.

### "Cannot Connect" when printing

- Ensure the printer is online and ready in Windows
- Check server logs for errors
- Confirm document format is supported (PDF works; JPEG/PNG may need direct raster support)
- If using a non-PDF printer, install Ghostscript and set `GS_PATH` environment variable

### Port 631 already in use

Windows sometimes runs its own IPP service. Disable it:

```powershell
# Stop and disable "Internet Printing Client" and "IP Print Provider" services
Stop-Service -Name "Spooler" -Force
Set-Service -Name "Spooler" -StartupType Disabled
# Or change the port in our config to something else and use custom URI (not recommended)
```

### Ghostscript required for non-PDF printers

If your printer only understands PCL/PS, install Ghostscript:
1. Download from https://ghostscript.com/releases/gsdnld.html
2. Install to `C:\Program Files\gs\gs10.03.0\bin\gswin64c.exe`
3. The server auto-detects it; verify `gswin64c -h` works in PATH

---

## How It Works

1. **mDNS**: Server broadcasts `_ipp._tcp` service with printer attributes
2. **Discovery**: iPhone sees printer in Print dialog
3. **IPP**: iPhone sends `Print-Job` request (HTTP POST with IPP binary body) to `http://<windows-ip>:631/ipp/print`
4. **Job Processing**: Server forwards job to Windows spooler
5. **Status**: iPhone can poll job status via `Get-Job-Attributes`

---

## Project Structure

```
airprint-server/
├── main.go              # Entry point (Windows only)
├── go.mod
├── internal/
│   ├── ipp/             # IPP HTTP server and request handlers
│   ├── mdns/            # mDNS advertising via zeroconf
│   ├── printer/         # Windows printer job queue and spooler
│   └── config/          # YAML configuration
├── pkg/
│   └── ippproto/        # Low-level IPP message encoding/decoding
└── docs/
    └── implementation-plan.md
```

---

## Development Status

### Implemented
- [x] Basic IPP server (Get-Printer-Attributes, Print-Job, Get-Job-Attributes, Cancel-Job)
- [x] mDNS advertising with AirPrint TXT records
- [x] Windows printer job queue with concurrent-safe access
- [x] Configuration via YAML
- [x] Web UI (basic status)

### TODO / In Progress
- [ ] PDF → EMF/RAW conversion for non-PDF printers (Ghostscript integration)
- [ ] Support additional IPP operations (Get-Jobs, Get-Printer-attributes-feed)
- [ ] Better printer capabilities detection (duplex, trays, resolutions)
- [ ] Print quality/copies/color options handling
- [ ] Web UI for job management/cancellation
- [ ] HTTPS/LDAP/authentication (optional)
- [ ] Standalone builds with embedded Ghostscript
- [ ] Service/daemon installation for Windows

---

## License

MIT License - see LICENSE file.

---

## Contributing

Pull requests welcome. Please open issues for bugs/feature requests.

---

## Acknowledgments

- Uses [github.com/grandcat/zeroconf](https://github.com/grandcat/zeroconf) for mDNS
- Uses [github.com/alexbrainman/printer](https://github.com/alexbrainman/printer) for Windows printing
- IPP implementation follows RFC 8010 (Internet Printing Protocol) and RFC 8011 (Model and Semantics)
