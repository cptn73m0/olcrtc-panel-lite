// Package traffic parses exact upstream payload events and schedules resets.
package traffic

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/juushimatsu/olcrtc-panel-lite/internal/store"
)

var trafficPattern = regexp.MustCompile(`^(?:(?:[0-9]{4}/[0-9]{2}/[0-9]{2} )?[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)? )?traffic: session=([^\s]+) addr=([^\s]+) in=([0-9]+) out=([0-9]+)$`)
var unitPattern = regexp.MustCompile(`^olcrtc-instance@([0-9]+)\.service$`)

// Event is one exact closed-stream payload event.
type Event struct {
	SessionID string
	Target    string
	Upload    int64
	Download  int64
}

// Parse extracts an official upstream traffic line.
func Parse(line string) (Event, error) {
	match := trafficPattern.FindStringSubmatch(strings.TrimSpace(line))
	if match == nil {
		return Event{}, errors.New("not a traffic event")
	}
	upload, err := strconv.ParseInt(match[3], 10, 64)
	if err != nil {
		return Event{}, err
	}
	download, err := strconv.ParseInt(match[4], 10, 64)
	if err != nil {
		return Event{}, err
	}
	return Event{SessionID: match[1], Target: match[2], Upload: upload, Download: download}, nil
}

// ResetDue reports whether the configured calendar boundary has passed.
func ResetDue(policy string, periodStart, now time.Time) bool {
	location := now.Location()
	start := periodStart.In(location)
	switch policy {
	case "daily":
		boundary := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		return start.Before(boundary)
	case "weekly":
		days := (int(now.Weekday()) + 6) % 7
		boundary := time.Date(now.Year(), now.Month(), now.Day()-days, 0, 0, 0, 0, location)
		return start.Before(boundary)
	case "monthly":
		boundary := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
		return start.Before(boundary)
	default:
		return false
	}
}

// Collector reads one journald stream for all instance units.
type Collector struct {
	store  *store.Store
	logger *slog.Logger
}

// NewCollector creates a collector.
func NewCollector(st *store.Store, logger *slog.Logger) *Collector {
	return &Collector{store: st, logger: logger}
}

// backlogWindow bounds how far back a restart replays the journal. A saved
// cursor older than this means the collector was down long enough for the
// backlog to be irrelevant; replaying days of events would flood the ledger.
const backlogWindow = 6 * time.Hour

// resumeArgs builds the journalctl arguments for the next collection run.
func resumeArgs(cursor string, lastEvent, now time.Time) []string {
	base := []string{"--follow", "--output=json", "--unit=olcrtc-instance@*.service"}
	if cursor != "" && now.Sub(lastEvent) <= backlogWindow {
		return append(base, "--after-cursor="+cursor)
	}
	return append(base, sinceArg(now.Add(-backlogWindow)))
}

func sinceArg(since time.Time) string {
	return "--since=@" + strconv.FormatInt(since.Unix(), 10)
}

type journalLine struct {
	Message string `json:"MESSAGE"`
	Cursor  string `json:"__CURSOR"`
	Unit    string `json:"_SYSTEMD_UNIT"`
	Time    string `json:"__REALTIME_TIMESTAMP"`
}

// Run follows the journal until the context ends. Any failure returns so the
// supervisor restarts the collector from the last committed cursor.
func (c *Collector) Run(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	cursor, eventAt, err := c.store.LastTrafficCursor(ctx)
	if err != nil {
		cursor, eventAt = "", time.Time{}
	}
	err = c.runJournal(ctx, resumeArgs(cursor, eventAt, time.Now()))
	if err == nil || ctx.Err() != nil || cursor == "" {
		return err
	}
	// The journal may have rotated past the saved cursor. Resume from the
	// last timestamp with overlap; cursor uniqueness prevents double counting.
	c.logger.Warn("journal cursor unusable, resuming by timestamp", "error", err)
	fallback := []string{"--follow", "--output=json", "--unit=olcrtc-instance@*.service", sinceArg(eventAt.Add(-time.Second))}
	return c.runJournal(ctx, fallback)
}

func (c *Collector) runJournal(ctx context.Context, args []string) error {
	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, "journalctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start journal collector: %w", err)
	}
	// stop kills the child and reaps it. Returning without this would leave
	// the journalctl child running and its pipe unread.
	stop := func() {
		cancel()
		_ = cmd.Wait()
	}
	scanner := bufio.NewScanner(stdout)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		var line journalLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		unitMatch := unitPattern.FindStringSubmatch(line.Unit)
		if unitMatch == nil {
			continue
		}
		event, err := Parse(line.Message)
		if err != nil {
			continue
		}
		id, _ := strconv.ParseInt(unitMatch[1], 10, 64)
		eventAt := time.Now()
		if micros, err := strconv.ParseInt(line.Time, 10, 64); err == nil {
			eventAt = time.UnixMicro(micros)
		}
		if _, err := c.store.ApplyTrafficEvent(runCtx, line.Cursor, id, event.SessionID, event.Target, event.Upload, event.Download, eventAt); err != nil {
			stop()
			if runCtx.Err() != nil {
				return nil
			}
			return fmt.Errorf("apply traffic event: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		stop()
		return err
	}
	return cmd.Wait()
}
