// Package ffmpeg provides FFmpeg integration for transcoding
package ffmpeg

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v4/examples/sip-server/internal/config"
)

// Transcoder handles FFmpeg-based media transcoding
type Transcoder struct {
	config     *config.Config
	logger     logging.LoggerFactory
	jobs       map[string]*TranscodeJob
	jobsMutex  sync.RWMutex
}

// TranscodeJob represents an active transcoding job
type TranscodeJob struct {
	ID          string
	InputFile   string
	OutputFile  string
	InputCodec  string
	OutputCodec string
	SampleRate  int
	Channels    int
	StartTime   time.Time
	Status      JobStatus
	Progress    float64
	Error       error
	cmd         *exec.Cmd
	mutex       sync.RWMutex
}

// JobStatus represents the status of a transcoding job
type JobStatus int

const (
	JobStatusPending JobStatus = iota
	JobStatusRunning
	JobStatusCompleted
	JobStatusFailed
	JobStatusCancelled
)

// String returns string representation of JobStatus
func (js JobStatus) String() string {
	switch js {
	case JobStatusPending:
		return "pending"
	case JobStatusRunning:
		return "running"
	case JobStatusCompleted:
		return "completed"
	case JobStatusFailed:
		return "failed"
	case JobStatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// TranscodeOptions contains options for transcoding
type TranscodeOptions struct {
	InputFile   string
	OutputFile  string
	InputCodec  string
	OutputCodec string
	SampleRate  int
	Channels    int
	Bitrate     int
	Quality     string
	StartTime   time.Duration
	Duration    time.Duration
}

// NewTranscoder creates a new FFmpeg transcoder
func NewTranscoder(cfg *config.Config, logger logging.LoggerFactory) *Transcoder {
	return &Transcoder{
		config: cfg,
		logger: logger,
		jobs:   make(map[string]*TranscodeJob),
	}
}

// IsFFmpegAvailable checks if FFmpeg is available
func (t *Transcoder) IsFFmpegAvailable() bool {
	if !t.config.FFmpeg.Enabled {
		return false
	}

	cmd := exec.Command(t.config.FFmpeg.Path, "-version")
	err := cmd.Run()
	return err == nil
}

// GetFFmpegVersion returns the FFmpeg version
func (t *Transcoder) GetFFmpegVersion() (string, error) {
	cmd := exec.Command(t.config.FFmpeg.Path, "-version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get FFmpeg version: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return lines[0], nil
	}

	return "unknown", nil
}

// TranscodeToRTP transcodes a media file to RTP-compatible format
func (t *Transcoder) TranscodeToRTP(inputFile, outputFile, codec string) (*TranscodeJob, error) {
	if !t.config.FFmpeg.Enabled {
		return nil, fmt.Errorf("FFmpeg is disabled")
	}

	options := TranscodeOptions{
		InputFile:   inputFile,
		OutputFile:  outputFile,
		OutputCodec: codec,
		SampleRate:  8000,  // Standard for telephony
		Channels:    1,     // Mono
		Quality:     "high",
	}

	return t.Transcode(options)
}

// Transcode starts a transcoding job with the given options
func (t *Transcoder) Transcode(options TranscodeOptions) (*TranscodeJob, error) {
	log := t.logger.NewLogger("ffmpeg-transcode")

	if !t.config.FFmpeg.Enabled {
		return nil, fmt.Errorf("FFmpeg is disabled")
	}

	// Generate job ID
	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())

	// Validate input file
	if _, err := os.Stat(options.InputFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("input file not found: %s", options.InputFile)
	}

	// Create output directory if needed
	if err := os.MkdirAll(filepath.Dir(options.OutputFile), 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create job
	job := &TranscodeJob{
		ID:          jobID,
		InputFile:   options.InputFile,
		OutputFile:  options.OutputFile,
		InputCodec:  options.InputCodec,
		OutputCodec: options.OutputCodec,
		SampleRate:  options.SampleRate,
		Channels:    options.Channels,
		StartTime:   time.Now(),
		Status:      JobStatusPending,
		Progress:    0.0,
	}

	// Build FFmpeg command
	args, err := t.buildFFmpegArgs(options)
	if err != nil {
		return nil, fmt.Errorf("failed to build FFmpeg command: %w", err)
	}

	job.cmd = exec.Command(t.config.FFmpeg.Path, args...)

	// Store job
	t.jobsMutex.Lock()
	t.jobs[jobID] = job
	t.jobsMutex.Unlock()

	log.Infof("Starting transcode job: %s (%s -> %s)", jobID, options.InputFile, options.OutputFile)

	// Start transcoding in goroutine
	go t.runTranscodeJob(job)

	return job, nil
}

// buildFFmpegArgs builds FFmpeg command arguments
func (t *Transcoder) buildFFmpegArgs(options TranscodeOptions) ([]string, error) {
	var args []string

	// Input file
	args = append(args, "-i", options.InputFile)

	// Output codec specific settings
	switch strings.ToUpper(options.OutputCodec) {
	case "PCMU":
		args = append(args, 
			"-acodec", "pcm_mulaw",
			"-ar", fmt.Sprintf("%d", options.SampleRate),
			"-ac", fmt.Sprintf("%d", options.Channels),
			"-f", "mulaw",
		)
	case "PCMA":
		args = append(args, 
			"-acodec", "pcm_alaw",
			"-ar", fmt.Sprintf("%d", options.SampleRate),
			"-ac", fmt.Sprintf("%d", options.Channels),
			"-f", "alaw",
		)
	case "OPUS":
		args = append(args, 
			"-acodec", "libopus",
			"-ar", fmt.Sprintf("%d", options.SampleRate),
			"-ac", fmt.Sprintf("%d", options.Channels),
			"-b:a", "64k",
		)
	case "L16":
		args = append(args, 
			"-acodec", "pcm_s16be",
			"-ar", fmt.Sprintf("%d", options.SampleRate),
			"-ac", fmt.Sprintf("%d", options.Channels),
			"-f", "s16be",
		)
	case "WAV":
		args = append(args, 
			"-acodec", "pcm_s16le",
			"-ar", fmt.Sprintf("%d", options.SampleRate),
			"-ac", fmt.Sprintf("%d", options.Channels),
		)
	default:
		return nil, fmt.Errorf("unsupported output codec: %s", options.OutputCodec)
	}

	// Quality settings
	if options.Quality == "high" {
		args = append(args, "-q:a", "0")
	} else if options.Quality == "low" {
		args = append(args, "-q:a", "9")
	}

	// Time range
	if options.StartTime > 0 {
		args = append(args, "-ss", options.StartTime.String())
	}
	if options.Duration > 0 {
		args = append(args, "-t", options.Duration.String())
	}

	// Overwrite output file
	args = append(args, "-y")

	// Output file
	args = append(args, options.OutputFile)

	return args, nil
}

// runTranscodeJob runs a transcoding job
func (t *Transcoder) runTranscodeJob(job *TranscodeJob) {
	log := t.logger.NewLogger("ffmpeg-job")

	job.mutex.Lock()
	job.Status = JobStatusRunning
	job.mutex.Unlock()

	log.Debugf("Running FFmpeg command: %s %s", t.config.FFmpeg.Path, strings.Join(job.cmd.Args[1:], " "))

	// Start the command
	var stderr bytes.Buffer
	job.cmd.Stderr = &stderr

	err := job.cmd.Run()

	job.mutex.Lock()
	if err != nil {
		job.Status = JobStatusFailed
		job.Error = fmt.Errorf("FFmpeg error: %w, stderr: %s", err, stderr.String())
		log.Errorf("Transcode job failed: %s, error: %v", job.ID, job.Error)
	} else {
		job.Status = JobStatusCompleted
		job.Progress = 100.0
		log.Infof("Transcode job completed: %s", job.ID)
	}
	job.mutex.Unlock()
}

// CancelJob cancels a running transcoding job
func (t *Transcoder) CancelJob(jobID string) error {
	t.jobsMutex.RLock()
	job, exists := t.jobs[jobID]
	t.jobsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job.mutex.Lock()
	defer job.mutex.Unlock()

	if job.Status == JobStatusRunning && job.cmd != nil {
		if err := job.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		job.Status = JobStatusCancelled
	}

	return nil
}

// GetJob returns a transcoding job by ID
func (t *Transcoder) GetJob(jobID string) (*TranscodeJob, bool) {
	t.jobsMutex.RLock()
	defer t.jobsMutex.RUnlock()
	job, exists := t.jobs[jobID]
	return job, exists
}

// GetJobs returns all transcoding jobs
func (t *Transcoder) GetJobs() map[string]*TranscodeJob {
	t.jobsMutex.RLock()
	defer t.jobsMutex.RUnlock()

	jobs := make(map[string]*TranscodeJob)
	for k, v := range t.jobs {
		jobs[k] = v
	}
	return jobs
}

// GetJobStats returns job statistics
func (t *Transcoder) GetJobStats() map[string]interface{} {
	t.jobsMutex.RLock()
	defer t.jobsMutex.RUnlock()

	stats := map[string]interface{}{
		"total_jobs": len(t.jobs),
		"ffmpeg_enabled": t.config.FFmpeg.Enabled,
		"ffmpeg_path": t.config.FFmpeg.Path,
	}

	statusCounts := make(map[string]int)
	for _, job := range t.jobs {
		job.mutex.RLock()
		statusCounts[job.Status.String()]++
		job.mutex.RUnlock()
	}
	stats["status_counts"] = statusCounts

	return stats
}

// CleanupCompletedJobs removes completed jobs older than the specified duration
func (t *Transcoder) CleanupCompletedJobs(maxAge time.Duration) {
	log := t.logger.NewLogger("ffmpeg-cleanup")
	cutoff := time.Now().Add(-maxAge)

	t.jobsMutex.Lock()
	defer t.jobsMutex.Unlock()

	for jobID, job := range t.jobs {
		job.mutex.RLock()
		shouldCleanup := (job.Status == JobStatusCompleted || job.Status == JobStatusFailed || job.Status == JobStatusCancelled) &&
			job.StartTime.Before(cutoff)
		job.mutex.RUnlock()

		if shouldCleanup {
			delete(t.jobs, jobID)
			log.Debugf("Cleaned up job: %s", jobID)
		}
	}
}

// ConvertToSIPCodec converts a media file to a SIP-compatible codec
func (t *Transcoder) ConvertToSIPCodec(inputFile, codec string) (string, error) {
	if !t.config.FFmpeg.Enabled {
		return "", fmt.Errorf("FFmpeg is disabled")
	}

	// Generate output filename
	ext := "raw"
	switch strings.ToUpper(codec) {
	case "PCMU":
		ext = "ulaw"
	case "PCMA":
		ext = "alaw"
	case "WAV":
		ext = "wav"
	case "OPUS":
		ext = "opus"
	}

	outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + "_" + strings.ToLower(codec) + "." + ext

	job, err := t.TranscodeToRTP(inputFile, outputFile, codec)
	if err != nil {
		return "", err
	}

	// Wait for completion (blocking)
	for {
		job.mutex.RLock()
		status := job.Status
		jobErr := job.Error
		job.mutex.RUnlock()

		if status == JobStatusCompleted {
			return outputFile, nil
		} else if status == JobStatusFailed {
			return "", fmt.Errorf("transcoding failed: %w", jobErr)
		}

		time.Sleep(100 * time.Millisecond)
	}
}