//go:build darwin

package state

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"time"
)

// Sample is one line of samples.jsonl: a rule measurement or, with Rule
// "_disk", a free space reading.
type Sample struct {
	TS         time.Time `json:"ts"`
	Rule       string    `json:"rule"`
	Allocated  int64     `json:"allocated,omitempty"`
	Apparent   int64     `json:"apparent,omitempty"`
	CloudOnly  int64     `json:"cloud_only,omitempty"`
	Units      int       `json:"units,omitempty"`
	UsableFree int64     `json:"usable_free,omitempty"`
	Total      int64     `json:"total,omitempty"`
}

// DiskSampleRule is the Rule value of free space readings.
const DiskSampleRule = "_disk"

// AppendSamples adds lines to samples.jsonl.
func (d Dir) AppendSamples(samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, s := range samples {
		if err := enc.Encode(s); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(d.Path("samples.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}
	return f.Sync()
}

// ReadSamples returns every sample in file order. Malformed lines are skipped.
func (d Dir) ReadSamples() ([]Sample, error) {
	f, err := os.Open(d.Path("samples.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Sample
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var s Sample
		if json.Unmarshal(sc.Bytes(), &s) == nil {
			out = append(out, s)
		}
	}
	return out, sc.Err()
}

// CompactSamples drops samples older than cutoff by rewriting the file
// through a temporary name. Callers only run it while the disk is healthy.
func (d Dir) CompactSamples(cutoff time.Time) error {
	all, err := d.ReadSamples()
	if err != nil || all == nil {
		return err
	}
	kept := all[:0]
	for _, s := range all {
		if !s.TS.Before(cutoff) {
			kept = append(kept, s)
		}
	}
	if len(kept) == len(all) {
		return nil
	}
	tmp, err := os.CreateTemp(string(d), "samples-*")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(tmp)
	for _, s := range kept {
		if err := enc.Encode(s); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), d.Path("samples.jsonl"))
}
