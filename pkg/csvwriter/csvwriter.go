package csvwriter

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"
)

type CSVFile struct {
	f   *os.File
	csv *csv.Writer
	gz  *gzip.Writer
}

func NewCSVFile(fname string, fields []string) (*CSVFile, error) {
	f, err := os.OpenFile(fname, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	gz := gzip.NewWriter(f)
	csv := csv.NewWriter(gz)
	c := &CSVFile{
		f:   f,
		gz:  gz,
		csv: csv,
	}

	// If file is empty, write header.
	if stat, err := f.Stat(); err == nil && stat.Size() == 0 {
		if err := c.WriteRow(fields); err != nil {
			return nil, fmt.Errorf("failed to write header to csv: %w", err)
		}
	}

	return c, nil
}

func (c *CSVFile) WriteRow(row []string) error {
	return c.csv.Write(row)
}

func (c *CSVFile) Flush() error {
	c.csv.Flush()
	c.gz.Flush()
	return c.f.Sync()
}

func (c *CSVFile) Close() error {
	if err := c.Flush(); err != nil {
		return err
	}

	return c.f.Close()
}

type CSVWriter struct {
	ch             chan []string
	rotateInterval RotationInterval
	fnamePrefix    string
	path           string
	fields         []string
	file           *CSVFile
}

type RotationInterval int

const (
	IntervalHourly RotationInterval = iota
	IntervalDaily
)

func genFname(prefix string, interval RotationInterval) string {
	switch interval {
	case IntervalHourly:
		return fmt.Sprintf("%s_%s.csv.gz", prefix, time.Now().Format("2006-01-02_15"))
	case IntervalDaily:
		return fmt.Sprintf("%s_%s.csv.gz", prefix, time.Now().Format("2006-01-02"))
	default:
		return ""
	}
}

func NewCSVWriter(
	ctx context.Context,
	path, fnamePrefix string,
	fields []string,
	rotateInterval RotationInterval,
) (*CSVWriter, error) {
	w := &CSVWriter{
		ch:             make(chan []string, 100),
		path:           path,
		fnamePrefix:    fnamePrefix,
		fields:         fields,
		rotateInterval: rotateInterval,
	}

	go w.run(ctx)

	return w, nil
}

func isNewInterval(last, current time.Time, interval RotationInterval) bool {
	if last.IsZero() {
		return true
	}
	switch interval {
	case IntervalHourly:
		return last.Hour() != current.Hour()
	case IntervalDaily:
		return last.Day() != current.Day()
	default:
		return false
	}
}

func (w *CSVWriter) run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var (
		lastRun time.Time
		cntErr  int
	)
	for {
		if isNewInterval(lastRun, time.Now(), w.rotateInterval) {
			if w.file != nil {
				if err := w.file.Close(); err != nil {
					log.Printf("failed to close file: %v\n", err)
				}
			}

			f, err := NewCSVFile(genFname(w.fnamePrefix, w.rotateInterval), w.fields)
			if err != nil {
				if cntErr > 5 {
					log.Fatalf("failed to create new file 5 times: %v", err)
				}
				log.Printf("failed to create new file: %v\n", err)
				time.Sleep(2 * time.Duration(cntErr) * time.Second)
				cntErr++
				continue
			}
			cntErr = 0

			w.file = f
			lastRun = time.Now()
		}

		select {
		case <-ctx.Done():
			w.file.Close()
			return
		case <-ticker.C:
			if err := w.file.Flush(); err != nil {
				log.Printf("failed to flush file: %v\n", err)
			}
		case row := <-w.ch:
			if err := w.file.WriteRow(row); err != nil {
				log.Printf("failed to write row: %v\n", err)
			}
		}
	}
}

func (w *CSVWriter) WriteRow(row []string) {
	w.ch <- row
}
