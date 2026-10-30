package config

import (
	"crypto/tls"
	"testing"
)

func TestRedisDefaultsArePlainAndPasswordless(t *testing.T) {
	t.Setenv("SNIP_REDIS_PASSWORD", "")
	t.Setenv("SNIP_REDIS_TLS", "")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RedisPassword != "" || c.RedisTLS {
		t.Fatalf("got password %q, TLS %v; want none, off", c.RedisPassword, c.RedisTLS)
	}
	if c.RedisTLSConfig() != nil {
		t.Fatal("TLS config without SNIP_REDIS_TLS: want nil (plain TCP)")
	}
}

func TestRedisPasswordAndTLS(t *testing.T) {
	t.Setenv("SNIP_REDIS_PASSWORD", "s3cret")
	t.Setenv("SNIP_REDIS_TLS", "true")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RedisPassword != "s3cret" {
		t.Fatalf("password = %q, want s3cret", c.RedisPassword)
	}
	tc := c.RedisTLSConfig()
	if tc == nil {
		t.Fatal("SNIP_REDIS_TLS=true: want a TLS config")
	}
	if tc.InsecureSkipVerify {
		t.Fatal("the server certificate must be verified")
	}
	if tc.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS 1.2", tc.MinVersion)
	}
}

func TestRedisTLSOff(t *testing.T) {
	t.Setenv("SNIP_REDIS_TLS", "false")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RedisTLS || c.RedisTLSConfig() != nil {
		t.Fatal("SNIP_REDIS_TLS=false: want plain TCP")
	}
}

func TestMalformedRedisTLSStopsStartup(t *testing.T) {
	t.Setenv("SNIP_REDIS_TLS", "yes please")
	if _, err := Load(); err == nil {
		t.Fatal("want an error for SNIP_REDIS_TLS=\"yes please\"")
	}
}
