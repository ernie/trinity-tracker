package collector

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"
)

// RawLogTailer streams raw log lines without parsing
type RawLogTailer struct {
	path     string
	file     *os.File
	position int64
	Lines    chan string
	Errors   chan error
	done     chan struct{}
}

// NewRawLogTailer creates a new raw log tailer
func NewRawLogTailer(path string) *RawLogTailer {
	return &RawLogTailer{
		path:   path,
		Lines:  make(chan string, 100),
		Errors: make(chan error, 10),
		done:   make(chan struct{}),
	}
}

// ReadLastNLines reads the last N lines from the log file
func (t *RawLogTailer) ReadLastNLines(n int) ([]string, error) {
	file, err := os.Open(t.path)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	fileSize := stat.Size()
	if fileSize == 0 {
		return []string{}, nil
	}

	// Read file backwards to find line boundaries
	const blockSize = 4096
	var lines []string
	var partial string
	position := fileSize

	for position > 0 && len(lines) < n {
		// Calculate read position
		readSize := int64(blockSize)
		if readSize > position {
			readSize = position
		}
		position -= readSize

		// Read block
		buf := make([]byte, readSize)
		_, err := file.ReadAt(buf, position)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("reading block: %w", err)
		}

		// Process block backwards
		content := string(buf) + partial
		partial = ""

		// Split into lines
		for i := len(content) - 1; i >= 0; i-- {
			if content[i] == '\n' {
				line := content[i+1:]
				if line != "" {
					lines = append(lines, line)
					if len(lines) >= n {
						break
					}
				}
				content = content[:i]
			}
		}

		// Remaining content becomes partial (incomplete line at start of block)
		if len(lines) < n {
			partial = content
		}
	}

	// Don't forget the first line if we reached beginning of file
	if partial != "" && len(lines) < n {
		lines = append(lines, partial)
	}

	// Reverse to get chronological order
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	return lines, nil
}

// Start begins tailing the log file from the current end
func (t *RawLogTailer) Start() error {
	file, err := os.Open(t.path)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	t.file = file

	// Seek to end to only process new lines
	pos, err := t.file.Seek(0, io.SeekEnd)
	if err != nil {
		t.file.Close()
		return fmt.Errorf("seeking to end: %w", err)
	}
	t.position = pos

	go t.tailLoop()
	return nil
}

// Stop stops the tailer
func (t *RawLogTailer) Stop() {
	close(t.done)
	if t.file != nil {
		t.file.Close()
	}
}

// tailLoop continuously reads new content from the log
func (t *RawLogTailer) tailLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			if err := t.readNewContent(); err != nil {
				select {
				case t.Errors <- err:
				default:
				}
			}
		}
	}
}

// readNewContent reads any new content since last read
func (t *RawLogTailer) readNewContent() error {
	stat, err := t.file.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	// Handle copytruncate: file size smaller than position
	if stat.Size() < t.position {
		t.position = 0
		if _, err := t.file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seeking to start after truncate: %w", err)
		}
	}

	// No new content
	if stat.Size() == t.position {
		return nil
	}

	// Read new content
	reader := bufio.NewReader(t.file)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			// Partial line - don't advance position past it
			break
		}
		if err != nil {
			return fmt.Errorf("reading line: %w", err)
		}

		// Update position
		t.position += int64(len(line))

		// Trim newline and send
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if line != "" {
			select {
			case t.Lines <- line:
			default:
				// Channel full, drop line
			}
		}
	}

	return nil
}
