package main

import "testing"

func TestParseArgs(t *testing.T) {
	cfg, err := parseArgs([]string{"--nofile", "16384", "--", "sh", "-c", "ulimit -n"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}
	if cfg.noFile != 16384 {
		t.Fatalf("noFile = %d, want 16384", cfg.noFile)
	}
	if len(cfg.command) != 3 || cfg.command[0] != "sh" {
		t.Fatalf("command = %v, want shell command", cfg.command)
	}
}

func TestParseArgsRejectsInvalidNoFile(t *testing.T) {
	tests := [][]string{
		{"--", "sh"},
		{"--nofile", "0", "--", "sh"},
		{"--nofile", "abc", "--", "sh"},
		{"--nofile", "16384"},
	}
	for _, tc := range tests {
		if _, err := parseArgs(tc); err == nil {
			t.Fatalf("parseArgs(%v) expected error", tc)
		}
	}
}
