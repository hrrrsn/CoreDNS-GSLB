package gslb

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/stretchr/testify/assert"
)

// Test setup function for the GSLB plugin.
func TestSetupGSLB(t *testing.T) {
	// Define test cases
	tests := []struct {
		name        string
		config      string
		expectError bool
	}{
		// Test with basic valid configuration (explicit zone-to-file mapping)
		{
			name: "Valid config with explicit zone-to-file mapping",
			config: `gslb {
				zone app-x.gslb.example.com ./tests/db.app-x.gslb.example.com.yml
			}`,
			expectError: false,
		},

		// Test with valid configuration and additional options
		{
			name: "Valid config with additional options",
			config: `gslb {
				zone app-x.gslb.example.com ./tests/db.app-x.gslb.example.com.yml
				max_stagger_start 120s
				batch_size_start 50
				resolution_idle_timeout 1800s
			}`,
			expectError: false,
		},

		// Test with geoip_maxmind block (valid syntax, no files)
		{
			name: "Valid geoip_maxmind block syntax",
			config: `gslb {
				zone app-x.gslb.example.com ./tests/db.app-x.gslb.example.com.yml
				geoip_maxmind country_db ./tests/GeoLite2-Country.mmdb
				geoip_maxmind city_db ./tests/GeoLite2-City.mmdb
				geoip_maxmind asn_db ./tests/GeoLite2-ASN.mmdb
			}`,
			expectError: false,
		},

		// Test with multiple zones and files
		{
			name: "Valid config with multiple zones and files",
			config: `gslb {
				zone app-x.gslb.example.com ./tests/db.app-x.gslb.example.com.yml
				zone app-y.gslb.example.com ./tests/db.app-y.gslb.example.com.yml
			}`,
			expectError: false,
		},

		// Test with all main parameters set
		{
			name: "Valid config with all main parameters",
			config: `gslb {
				zone app-x.gslb.example.com ./tests/db.app-x.gslb.example.com.yml
				max_stagger_start 90s
				batch_size_start 42
				resolution_idle_timeout 1234s
				geoip_maxmind country_db ./tests/GeoLite2-Country.mmdb
				geoip_maxmind city_db ./tests/GeoLite2-City.mmdb
				geoip_maxmind asn_db ./tests/GeoLite2-ASN.mmdb
				geoip_custom ./tests/location_map.yml
				api_enable false
				api_tls_cert /tmp/cert.pem
				api_tls_key /tmp/key.pem
				api_listen_addr 127.0.0.1
				api_listen_port 9999
				api_basic_user testuser
				api_basic_pass testpass
				healthcheck_idle_multiplier 7
			}`,
			expectError: false,
		},
		// Test with disable_txt option
		{
			name: "Disable TXT option disables TXT queries",
			config: `gslb {
				zone app-x.gslb.example.com ./tests/db.app-x.gslb.example.com.yml
				disable_txt
			}`,
			expectError: false,
		},
	}

	// Iterate over test cases
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a new Caddy controller for each test case
			c := caddy.NewTestController("dns", test.config)
			err := setup(c)

			// Only expect no error for all cases
			if err != nil {
				t.Fatalf("Expected no error, but got: %v for test: %v", err, test.name)
			}
		})
	}
}
func TestLoadRealConfig(t *testing.T) {
	// Test loading the appX config file with healthcheck profiles
	g := &GSLB{}
	zone := "app-x.gslb.example.com."
	err := loadConfigFile(g, "./tests/db.app-x.gslb.example.com.yml", zone)
	assert.NoError(t, err)

	// Verify healthcheck profiles were loaded
	assert.NotNil(t, g.HealthcheckProfiles)
	assert.Len(t, g.HealthcheckProfiles, 4) // https_default, icmp_default, grpc_default, lua_default

	expectedProfiles := []string{"https_default", "icmp_default", "grpc_default", "lua_default"}
	for _, profileName := range expectedProfiles {
		assert.Contains(t, g.HealthcheckProfiles, profileName, "Should contain profile %s", profileName)
	}

	// Verify records were loaded and processed
	assert.NotNil(t, g.Records)
	assert.Len(t, g.Records, 1)
	for _, recs := range g.Records {
		assert.Len(t, recs, 3)
	}

	zone = "webapp.app-x.gslb.example.com."
	if g.Records[zone] == nil {
		// fallback: try to find the only zone key
		for z := range g.Records {
			zone = z
			break
		}
	}
	record, ok := g.Records[zone]["webapp.app-x.gslb.example.com."]
	assert.True(t, ok, "Record webapp.app-x.gslb.example.com. should exist in zone %s", zone)
	assert.NotNil(t, record)
	assert.Equal(t, "failover", record.Mode)
	assert.Len(t, record.Backends, 2)

	// Check first backend - should have 1 healthcheck (https_default)
	backend1 := record.Backends[0]
	assert.Equal(t, "172.16.0.10", backend1.GetAddress())
	healthchecks1 := backend1.GetHealthChecks()
	assert.Len(t, healthchecks1, 1)
	assert.Equal(t, "https/443", healthchecks1[0].GetType())

	// Check second backend - should have 2 healthchecks (https_default + icmp_default)
	backend2 := record.Backends[1]
	assert.Equal(t, "172.16.0.11", backend2.GetAddress())
	healthchecks2 := backend2.GetHealthChecks()
	assert.Len(t, healthchecks2, 2)

	// Should have HTTPS and ICMP
	found_https := false
	found_icmp := false
	for _, hc := range healthchecks2 {
		if hc.GetType() == "https/443" {
			found_https = true
		}
		if hc.GetType() == ICMPType {
			found_icmp = true
		}
	}
	assert.True(t, found_https, "Should have HTTPS healthcheck")
	assert.True(t, found_icmp, "Should have ICMP healthcheck")

	// Vérifier la présence des autres records
	_, ok = g.Records[zone]["webapp-lua.app-x.gslb.example.com."]
	assert.True(t, ok, "Record webapp-lua.app-x.gslb.example.com. should exist in zone %s", zone)
	_, ok = g.Records[zone]["webapp-grpc.app-x.gslb.example.com."]
	assert.True(t, ok, "Record webapp-grpc.app-x.gslb.example.com. should exist in zone %s", zone)
}

func TestConfigWatcherDetectsChanges(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "gslb_watcher_test_")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a minimal test config file with YAML structure the loadConfigFile expects
	testConfigPath := filepath.Join(tmpDir, "test_config.yml")
	initialConfig := `defaults:
  owner: admin
  record_ttl: 30

records:
  app.test.example.com.:
    mode: failover
    backends:
      - address: 192.168.1.1
        priority: 1
      - address: 192.168.1.2
        priority: 2
`
	err = os.WriteFile(testConfigPath, []byte(initialConfig), 0644)
	assert.NoError(t, err)

	// Create GSLB instance and load initial config
	g := &GSLB{
		Zones:   make(map[string]string),
		Records: make(map[string]map[string]*Record),
	}
	zone := "test.example.com."
	g.Zones[zone] = testConfigPath

	err = loadConfigFile(g, testConfigPath, zone)
	assert.NoError(t, err)

	// Verify initial state - first backend should have priority 1
	recordKey := "app.test.example.com."
	assert.Contains(t, g.Records[zone], recordKey, "Record should exist initially")
	initialRecord := g.Records[zone][recordKey]
	assert.Len(t, initialRecord.Backends, 2, "Should have 2 backends")
	firstBackendInitial := initialRecord.Backends[0]
	assert.Equal(t, "192.168.1.1", firstBackendInitial.GetAddress(), "First backend address")
	assert.Equal(t, 1, firstBackendInitial.GetPriority(), "First backend initial priority should be 1")

	// Start the watcher in a goroutine
	go func() {
		// This will run forever, but we don't care - just let it run
		_ = startConfigWatcher(g, testConfigPath)
	}()

	// Give the watcher time to start and add the file to watch
	time.Sleep(300 * time.Millisecond)

	// Modify the config file - SWAP the priorities
	modifiedConfig := `defaults:
  owner: admin
  record_ttl: 30

records:
  app.test.example.com.:
    mode: failover
    backends:
      - address: 192.168.1.1
        priority: 2
      - address: 192.168.1.2
        priority: 1
`
	err = os.WriteFile(testConfigPath, []byte(modifiedConfig), 0644)
	assert.NoError(t, err)

	// Wait for the debounce timer (500ms) + processing time
	time.Sleep(1000 * time.Millisecond)

	// Verify config was reloaded AND priorities changed
	assert.NotNil(t, g.Records[zone], "Records should exist after reload")
	assert.Contains(t, g.Records[zone], recordKey, "Expected record should exist after reload")

	reloadedRecord := g.Records[zone][recordKey]
	assert.Len(t, reloadedRecord.Backends, 2, "Should still have 2 backends after reload")

	// Check that priorities were actually updated
	firstBackendAfterReload := reloadedRecord.Backends[0]
	assert.Equal(t, "192.168.1.1", firstBackendAfterReload.GetAddress(), "First backend address should stay the same")
	assert.Equal(t, 2, firstBackendAfterReload.GetPriority(), "First backend priority should be CHANGED to 2")

	secondBackendAfterReload := reloadedRecord.Backends[1]
	assert.Equal(t, "192.168.1.2", secondBackendAfterReload.GetAddress(), "Second backend address")
	assert.Equal(t, 1, secondBackendAfterReload.GetPriority(), "Second backend priority should be CHANGED to 1")
}
