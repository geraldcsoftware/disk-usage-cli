//go:build darwin

package config

import (
	"testing"
	"time"
)

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		in      string
		want    ByteSize
		wantErr bool
	}{
		{"6GB", 6_000_000_000, false},
		{"6gb", 6_000_000_000, false},
		{"5GiB", 5 << 30, false},
		{"128MB", 128_000_000, false},
		{"1.5GB", 1_500_000_000, false},
		{"0", 0, false},
		{"512", 512, false},
		{"10G", 0, true},
		{"10M", 0, true},
		{"", 0, true},
		{"GB", 0, true},
		{"-1GB", 0, true},
		{"1XB", 0, true},
	}
	for _, c := range cases {
		got, err := ParseByteSize(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseByteSize(%q) error = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("ParseByteSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"30m", 30 * time.Minute, false},
		{"6h", 6 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"1.5d", 36 * time.Hour, false},
		{"0", 0, false},
		{"-1h", 0, true},
		{"7days", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := ParseDuration(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseDuration(%q) error = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if got.Std() != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got.Std(), c.want)
		}
	}
}

func TestParseThreshold(t *testing.T) {
	pct, err := ParseThreshold("15%")
	if err != nil || !pct.IsPercent || pct.Percent != 15 {
		t.Fatalf("ParseThreshold(15%%) = %+v, %v", pct, err)
	}
	abs, err := ParseThreshold("25GB")
	if err != nil || abs.IsPercent || abs.Bytes != 25_000_000_000 {
		t.Fatalf("ParseThreshold(25GB) = %+v, %v", abs, err)
	}
	for _, bad := range []string{"0%", "100%", "25G", "", "abc"} {
		if _, err := ParseThreshold(bad); err == nil {
			t.Errorf("ParseThreshold(%q) accepted", bad)
		}
	}
}

func TestThresholdComparisons(t *testing.T) {
	const total = 200_000_000_000
	pct, _ := ParseThreshold("15%") // limit 30 GB
	if !pct.Below(29_000_000_000, total) || pct.Below(30_000_000_000, total) {
		t.Error("Below at 15% of 200 GB should trigger under 30 GB only")
	}
	if !pct.Above(31_000_000_000, total) || pct.Above(30_000_000_000, total) {
		t.Error("Above at 15% of 200 GB should trigger over 30 GB only")
	}
	list := Thresholds{pct, must(ParseThreshold("25GB"))}
	if !list.AnyBelow(26_000_000_000, total) {
		t.Error("26 GB is below the 30 GB percentage limit")
	}
	if list.AnyBelow(31_000_000_000, total) {
		t.Error("31 GB is below neither entry")
	}
	if !list.AllAbove(31_000_000_000, total) {
		t.Error("31 GB is above both entries")
	}
	if list.AllAbove(28_000_000_000, total) {
		t.Error("28 GB is not above the 30 GB entry")
	}
}

func must(t Threshold, err error) Threshold {
	if err != nil {
		panic(err)
	}
	return t
}
