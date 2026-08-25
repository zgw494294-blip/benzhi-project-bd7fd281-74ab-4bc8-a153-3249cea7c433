package main

import "testing"

func TestExplicitLoopbackAddress(t *testing.T) {
	cfg, err := parseConfig([]string{"-addr=127.0.0.1:19099", "-selfcheck"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:19099" || !cfg.SelfCheck {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
func TestRejectsPublicBind(t *testing.T) {
	if _, err := parseConfig([]string{"-addr=0.0.0.0:19081"}); err == nil {
		t.Fatal("expected error")
	}
}
