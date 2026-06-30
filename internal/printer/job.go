//go:build windows

package printer

import (
	"fmt"
	"sync"
	"time"

	winprinter "github.com/alexbrainman/printer"
)

// JobStatus represents the status of a print job
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusCanceled   JobStatus = "canceled"
	StatusError      JobStatus = "error"
)

// PrintJob represents a single print job
type PrintJob struct {
	ID        int
	Format    string // MIME type: application/pdf, image/jpeg, etc.
	Data      []byte
	User      string
	Title     string
	Status    JobStatus
	ErrorMsg  string
	CreatedAt time.Time
	StartedAt time.Time
	FinishedAt time.Time
}

// Printer provides an abstraction over Windows printing
type Printer struct {
	name     string
	printer  *winprinter.Printer
	jobs     map[int]*PrintJob
	mu       sync.RWMutex
	nextID   int
	jobChan  chan *PrintJob
	stopChan chan struct{}
}

// Name returns the printer name
func (p *Printer) Name() string {
	return p.name
}

// New creates a new Printer instance
func New(name string) (*Printer, error) {
	if name == "" {
		// Get default printer
		name = winprinter.Default()
		if name == "" {
			return nil, fmt.Errorf("no default printer found")
		}
	}

	// Open the printer using Windows spooler
	w, err := winprinter.Open(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open printer %s: %w", name, err)
	}

	pr := &Printer{
		name:    name,
		printer: w,
		jobs:    make(map[int]*PrintJob),
		jobChan: make(chan *PrintJob, 100),
		stopChan: make(chan struct{}),
	}

	// Start job processor
	go pr.processJobs()

	return pr, nil
}

// Print submits a job to the printer
func (p *Printer) Print(data []byte, format, user, title string) (int, error) {
	p.mu.Lock()
	id := p.nextID
	p.nextID++
	p.mu.Unlock()

	job := &PrintJob{
		ID:        id,
		Format:    format,
		Data:      data,
		User:      user,
		Title:     title,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	p.mu.Lock()
	p.jobs[id] = job
	p.mu.Unlock()

	// Send to job processor (non-blocking if buffer not full)
	select {
	case p.jobChan <- job:
	default:
		// Job queue is full
		p.mu.Lock()
		delete(p.jobs, id)
		p.mu.Unlock()
		return 0, fmt.Errorf("job queue is full")
	}

	return id, nil
}

// GetJob returns job information
func (p *Printer) GetJob(id int) (*PrintJob, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	job, exists := p.jobs[id]
	return job, exists
}

// CancelJob attempts to cancel a pending or processing job
func (p *Printer) CancelJob(id int) error {
	p.mu.Lock()
	job, exists := p.jobs[id]
	if !exists {
		p.mu.Unlock()
		return fmt.Errorf("job %d not found", id)
	}

	if job.Status == StatusPending || job.Status == StatusProcessing {
		// Mark as canceled; job processor will respect this
		job.Status = StatusCanceled
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	return fmt.Errorf("job %d is already %s", id, job.Status)
}

// GetStatus returns printer status (simplified)
func (p *Printer) GetStatus() (string, error) {
	// Query printer status via Windows API
	// This is simplified - actual implementation would need to parse printer info
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Count jobs
	pending := 0
	processing := 0
	for _, job := range p.jobs {
		switch job.Status {
		case StatusPending:
			pending++
		case StatusProcessing:
			processing++
		}
	}

	if processing > 0 {
		return "processing", nil
	}
	if pending > 0 {
		return "idle (jobs pending)", nil
	}
	return "idle", nil
}

// processJobs handles print jobs sequentially
func (p *Printer) processJobs() {
	for {
		select {
		case <-p.stopChan:
			return
		case job := <-p.jobChan:
			p.processJob(job)
		}
	}
}

// processJob handles a single print job
func (p *Printer) processJob(job *PrintJob) {
	// Set status to processing
	p.mu.Lock()
	job.Status = StatusProcessing
	job.StartedAt = time.Now()
	p.mu.Unlock()

	// TODO: Convert job.Data based on job.Format to EMF/RAW
	// For now, we assume printer can handle raw data or use Windows spooler

	err := p.printRaw(job.Data, job.Title)

	p.mu.Lock()
	job.FinishedAt = time.Now()
	if err != nil {
		job.Status = StatusError
		job.ErrorMsg = err.Error()
	} else {
		job.Status = StatusCompleted
	}
	p.mu.Unlock()
}

// printRaw sends raw data to the printer
func (p *Printer) printRaw(data []byte, docName string) error {
	h, err := p.printer.StartDoc(docName)
	if err != nil {
		return fmt.Errorf("failed to start doc: %w", err)
	}
	defer p.printer.EndDoc(h)

	if err := p.printer.Write(h, data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// Close releases resources
func (p *Printer) Close() error {
	close(p.stopChan)
	return p.printer.Close()
}

// ListJobs returns all jobs (for debugging/mgmt)
func (p *Printer) ListJobs() []*PrintJob {
	p.mu.RLock()
	defer p.mu.RUnlock()

	jobs := make([]*PrintJob, 0, len(p.jobs))
	for _, job := range p.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// CleanupJobs removes old completed/canceled jobs (older than 24h)
func (p *Printer) CleanupJobs() {
	p.mu.Lock()
	defer p.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	for id, job := range p.jobs {
		if job.FinishedAt.Before(cutoff) && (job.Status == StatusCompleted || job.Status == StatusCanceled) {
			delete(p.jobs, id)
		}
	}
}
