package ipp

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"

	"airprint-server/internal/printer"
	"airprint-server/pkg/ippproto"
)

// Server is an HTTP-based IPP server
type Server struct {
	addr    string
	port    int
	handler *Handler
	httpSrv *http.Server
	mu      sync.RWMutex
	running bool
}

// Handler processes IPP requests over HTTP
type Handler struct {
	printer       *printer.Printer
	printerAttrs  []ippproto.Attribute
}

// NewServer creates an IPP server
func NewServer(addr string, port int, pr *printer.Printer) *Server {
	h := &Handler{
		printer: pr,
	}
	h.initPrinterAttributes()

	s := &Server{
		addr:    addr,
		port:    port,
		handler: h,
	}
	return s
}

// Start begins listening for HTTP connections
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.addr, s.port)
	mux := http.NewServeMux()
	mux.HandleFunc("/ipp/print", s.handler.ServeIPP)
	// Also register a path based on printer name for compatibility
	printerNameEscaped := url.PathEscape(s.handler.printer.Name())
	mux.HandleFunc("/printers/"+printerNameEscaped, s.handler.ServeIPP)

	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	log.Printf("IPP HTTP server listening on %s", addr)
	return s.httpSrv.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.running = false
		if s.httpSrv != nil {
			s.httpSrv.Close()
		}
		log.Println("IPP server stopped")
	}
}

// ServeIPP handles an IPP request over HTTP
func (h *Handler) ServeIPP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read full body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Decode IPP request. We need to separate the IPP attributes from the document data.
	br := bytes.NewReader(body)
	req := &ippproto.Message{}
	if err := req.Decode(br); err != nil {
		log.Printf("Failed to decode IPP request: %v", err)
		http.Error(w, "Invalid IPP request", http.StatusBadRequest)
		return
	}

	// Remaining bytes after the IPP attributes are the document data (if any)
	consumed := len(body) - br.Len()
	req.DocumentData = body[consumed:]

	// Process the request
	resp := h.handleRequest(req)

	// Encode response
	var buf bytes.Buffer
	if err := resp.Encode(&buf); err != nil {
		log.Printf("Failed to encode IPP response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}

	// Write HTTP response
	w.Header().Set("Content-Type", "application/ipp")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// initPrinterAttributes initializes the printer's advertised capabilities
func (h *Handler) initPrinterAttributes() {
	// Basic attributes
	h.printerAttrs = []ippproto.Attribute{
		{Tag: ippproto.TagNameWithLanguage, Name: ippproto.AttrPrinterName, Value: h.printer.Name()},
		{Tag: ippproto.TagString, Name: ippproto.AttrPrinterLocation, Value: "Local"},
		{Tag: ippproto.TagString, Name: ippproto.AttrPrinterInfo, Value: "AirPrint Server for Windows"},
		{Tag: ippproto.TagInteger, Name: ippproto.AttrPrinterState, Value: int32(3)}, // idle
		{Tag: ippproto.TagString, Name: ippproto.AttrPrinterStateReasons, Value: "none"},
		{Tag: ippproto.TagInteger, Name: ippproto.AttrQueuedJobCount, Value: int32(0)},
		{Tag: ippproto.TagString, Name: "printer-uri-supported", Value: fmt.Sprintf("ipp://%s:631/ipp/print", getLocalIP())},
		{Tag: ippproto.TagString, Name: "uri-authentication-supported", Value: "none"},
		{Tag: ippproto.TagString, Name: "uri-security-supported", Value: "none"},
		{Tag: ippproto.TagInteger, Name: "printer-type", Value: int32(0x801002)}, // AirPrint
	}

	// Supported document formats (must include PDF for AirPrint)
	formats := []string{"application/pdf", "image/jpeg", "image/png", "application/postscript"}
	for _, f := range formats {
		h.printerAttrs = append(h.printerAttrs, ippproto.Attribute{
			Tag:   ippproto.TagString,
			Name:  ippproto.AttrDocumentFormatSupported,
			Value: f,
		})
	}

	// Color support
	h.printerAttrs = append(h.printerAttrs, ippproto.Attribute{
		Tag:   ippproto.TagBoolean,
		Name:  ippproto.AttrColorSupported,
		Value: true,
	})

	// Duplex support (true)
	h.printerAttrs = append(h.printerAttrs, ippproto.Attribute{
		Tag:   ippproto.TagBoolean,
		Name:  ippproto.AttrDuplexSupported,
		Value: true,
	})
}

// handleRequest routes IPP operations
func (h *Handler) handleRequest(req *ippproto.Message) *ippproto.Message {
	op := req.OperationID
	reqID := req.RequestID

	log.Printf("Received IPP operation 0x%04x from %s", op, req.Attributes) // Consider logging user

	switch op {
	case ippproto.OpGetPrinterAttributes:
		return h.handleGetPrinterAttributes(reqID)
	case ippproto.OpPrintJob:
		return h.handlePrintJob(req)
	case ippproto.OpGetJobAttributes:
		return h.handleGetJobAttributes(req)
	case ippproto.OpCancelJob:
		return h.handleCancelJob(req)
	default:
		log.Printf("Unsupported operation: 0x%04x", op)
		return ippproto.NewResponse(reqID, ippproto.StatusClientErrorBadRequest)
	}
}

// handleGetPrinterAttributes returns printer capabilities
func (h *Handler) handleGetPrinterAttributes(reqID uint32) *ippproto.Message {
	resp := ippproto.NewResponse(reqID, ippproto.StatusSuccessfulOK)
	for _, attr := range h.printerAttrs {
		resp.Attributes = append(resp.Attributes, attr)
	}
	return resp
}

// handlePrintJob accepts a print job
func (h *Handler) handlePrintJob(req *ippproto.Message) *ippproto.Message {
	var userName, docFormat string

	// Extract attributes
		for _, attr := range req.Attributes {
		switch attr.Name {
		case "requesting-user-name":
			if s, ok := attr.Value.(string); ok {
				userName = s
			}
		case "document-format":
			if s, ok := attr.Value.(string); ok {
				docFormat = s
			}
		}
	}

	if userName == "" {
		userName = "anonymous"
	}
	if docFormat == "" {
		docFormat = "application/pdf"
	}

	// Validate document format is supported
	supported := false
	for _, f := range []string{"application/pdf", "image/jpeg", "image/png"} { // duplicate list but okay
		if f == docFormat {
			supported = true
			break
		}
	}
	if !supported {
		log.Printf("Unsupported document format: %s", docFormat)
		return ippproto.NewResponse(req.RequestID, ippproto.StatusClientErrorDocumentFormatNotSupported)
	}

	// Ensure we have document data
	if len(req.DocumentData) == 0 {
		log.Println("PrintJob: no document data provided")
		return ippproto.NewResponse(req.RequestID, ippproto.StatusClientErrorNotPossible)
	}

	// Submit print job to printer backend
	jobID, err := h.printer.Print(req.DocumentData, docFormat, userName, "Untitled")
	if err != nil {
		log.Printf("Print job failed: %v", err)
		return ippproto.NewResponse(req.RequestID, ippproto.StatusServerErrorInternalError)
	}

	resp := ippproto.NewResponse(req.RequestID, ippproto.StatusSuccessfulOK)
	resp.AddAttribute(ippproto.TagInteger, "job-id", int32(jobID))
	resp.AddAttribute(ippproto.TagInteger, "job-state", int32(3)) // pending

	return resp
}

// handleGetJobAttributes returns status of a specific job
func (h *Handler) handleGetJobAttributes(req *ippproto.Message) *ippproto.Message {
	var jobID int32
	for _, attr := range req.Attributes {
		if attr.Name == "job-id" {
			if v, ok := attr.Value.(int32); ok {
				jobID = v
			}
		}
	}

	job, exists := h.printer.GetJob(int(jobID))
	if !exists {
		return ippproto.NewResponse(req.RequestID, ippproto.StatusClientErrorNotFound)
	}

	resp := ippproto.NewResponse(req.RequestID, ippproto.StatusSuccessfulOK)
	resp.AddAttribute(ippproto.TagInteger, "job-id", int32(job.ID))
	resp.AddAttribute(ippproto.TagString, "job-state", string(job.Status))
	resp.AddAttribute(ippproto.TagString, "job-name", job.Title)
	resp.AddAttribute(ippproto.TagString, "job-originating-user-name", job.User)

	return resp
}

// handleCancelJob cancels a job
func (h *Handler) handleCancelJob(req *ippproto.Message) *ippproto.Message {
	var jobID int32
	for _, attr := range req.Attributes {
		if attr.Name == "job-id" {
			if v, ok := attr.Value.(int32); ok {
				jobID = v
			}
		}
	}

	if jobID == 0 {
		return ippproto.NewResponse(req.RequestID, ippproto.StatusClientErrorBadRequest)
	}

	err := h.printer.CancelJob(int(jobID))
	if err != nil {
		return ippproto.NewResponse(req.RequestID, ippproto.StatusClientErrorNotPossible)
	}

	return ippproto.NewResponse(req.RequestID, ippproto.StatusSuccessfulOK)
}

// getLocalIP returns the local IP address for use in URI
func getLocalIP() string {
	// Attempt to determine local IP by connecting to a remote address
	// This is a simplified version
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "localhost"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
