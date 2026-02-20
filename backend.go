package gslb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/creasty/defaults"
)

// Backend represents an individual backend with health check settings.
type Backend struct {
	Fqdn            string               // Fully qualified domain name
	Description     string               // Description of the backend
	Address         string               // IP address or hostname
	Priority        int                  // Priority for load balancing
	Weight          int                  // Weight for weighted load balancing
	Enable          bool                 // Enable or disable the backend
	Tags            []string             // List of tags for filtering or grouping
	HealthChecks    []GenericHealthCheck `yaml:"healthchecks"` // Health check configurations
	Timeout         string               // Timeout for requests
	Alive           bool                 // Indicates if the backend is alive
	Country         string               // Country code for GeoIP
	City            string               // City name for GeoIP
	ASN             string               // ASN for GeoIP
	Location        string               // location
	Latitude        float64              // backend latitude for nearest routing
	Longitude       float64              // backend longitude for nearest routing
	CoordinatesSet  bool                 // indicates if latitude/longitude were provided
	LastHealthcheck time.Time            // Last time a healthcheck was launched
	ResponseTime    time.Duration        // Wall-clock duration of last health check run (used by fastest mode)
	mutex           sync.RWMutex
}

func (b *Backend) Lock() {
	b.mutex.Lock()
}

func (b *Backend) Unlock() {
	b.mutex.Unlock()
}

func (b *Backend) GetFqdn() string {
	return b.Fqdn
}

func (b *Backend) SetFqdn(fqdn string) {
	b.Fqdn = fqdn
}

func (b *Backend) GetDescription() string {
	return b.Description
}

func (b *Backend) GetAddress() string {
	return b.Address
}

func (b *Backend) GetPriority() int {
	return b.Priority
}

func (b *Backend) GetWeight() int {
	if b.Weight <= 0 {
		return 1
	}
	return b.Weight
}

func (b *Backend) IsEnabled() bool {
	return b.Enable
}

func (b *Backend) GetTags() []string {
	return b.Tags
}

func (b *Backend) GetHealthChecks() []GenericHealthCheck {
	return b.HealthChecks
}

func (b *Backend) GetTimeout() string {
	return b.Timeout
}

func (b *Backend) GetCountry() string {
	return b.Country
}

func (b *Backend) GetCity() string {
	return b.City
}

func (b *Backend) GetASN() string {
	return b.ASN
}

func (b *Backend) GetLocation() string {
	return b.Location
}

func (b *Backend) GetLatitude() float64 {
	return b.Latitude
}

func (b *Backend) GetLongitude() float64 {
	return b.Longitude
}

func (b *Backend) HasCoordinates() bool {
	return b.CoordinatesSet
}

func (b *Backend) GetResponseTime() time.Duration {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return b.ResponseTime
}

func (b *Backend) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw struct {
		Description  string        `yaml:"description" default:""`
		Address      string        `yaml:"address" default:"127.0.0.1"`
		Priority     int           `yaml:"priority" default:"0"`
		Weight       int           `yaml:"weight" default:"1"`
		Enable       bool          `yaml:"enable" default:"true"`
		Tags         []string      `yaml:"tags"`
		Timeout      string        `yaml:"timeout" default:"5s"`
		HealthChecks []HealthCheck `yaml:"healthchecks"`
		Country      string        `yaml:"country"`
		City         string        `yaml:"city"`
		ASN          string        `yaml:"asn"`
		Location     string        `yaml:"location"`
		Latitude     *float64      `yaml:"latitude"`
		Longitude    *float64      `yaml:"longitude"`
	}
	defaults.Set(&raw)
	if err := unmarshal(&raw); err != nil {
		return err
	}
	b.Description = raw.Description
	b.Address = raw.Address
	b.Priority = raw.Priority
	b.Weight = raw.Weight
	b.Enable = raw.Enable
	b.Tags = raw.Tags
	b.Timeout = raw.Timeout
	b.Country = raw.Country
	b.City = raw.City
	b.ASN = raw.ASN
	b.Location = raw.Location
	if (raw.Latitude == nil) != (raw.Longitude == nil) {
		return fmt.Errorf("backend %s: latitude and longitude must be set together", raw.Address)
	}
	if raw.Latitude != nil && raw.Longitude != nil {
		b.Latitude = *raw.Latitude
		b.Longitude = *raw.Longitude
		b.CoordinatesSet = true
	} else {
		b.CoordinatesSet = false
	}
	for _, hc := range raw.HealthChecks {
		specificHC, err := hc.ToSpecificHealthCheck()
		if err != nil {
			return fmt.Errorf("error converting healthcheck for backend %s: %w", b.Address, err)
		}
		b.HealthChecks = append(b.HealthChecks, specificHC)
	}
	return nil
}

// removeBackend stops the health check and performs cleanup for the backend
func (b *Backend) removeBackend() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	log.Infof("[%s] backend %s successfully removed", b.Fqdn, b.Address)
}

// updateBackend updates the settings of an existing backend
func (b *Backend) updateBackend(newBackend BackendInterface) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.Priority != newBackend.GetPriority() {
		log.Infof("[%s] backend %s updated, priority changed from %d to %d", b.Fqdn, b.Address, b.Priority, newBackend.GetPriority())
		b.Priority = newBackend.GetPriority()
	}

	if b.Weight != newBackend.GetWeight() {
		log.Infof("[%s] backend %s updated, weight changed from %d to %d", b.Fqdn, b.Address, b.Weight, newBackend.GetWeight())
		b.Weight = newBackend.GetWeight()
	}

	if b.Enable != newBackend.IsEnabled() {
		log.Infof("[%s] backend %s updated, enable changed from %v to %v", b.Fqdn, b.Address, b.Enable, newBackend.IsEnabled())
		b.Enable = newBackend.IsEnabled()
	}

	if b.Description != newBackend.GetDescription() {
		log.Infof("[%s] backend %s updated, description changed", b.Fqdn, b.Address)
		b.Description = newBackend.GetDescription()
	}

	if b.Timeout != newBackend.GetTimeout() {
		log.Infof("[%s] backend %s updated, timeout changed from %s to %s", b.Fqdn, b.Address, b.Timeout, newBackend.GetTimeout())
		b.Timeout = newBackend.GetTimeout()
	}

	if b.Country != newBackend.GetCountry() {
		log.Infof("[%s] backend %s updated, country changed from %s to %s", b.Fqdn, b.Address, b.Country, newBackend.GetCountry())
		b.Country = newBackend.GetCountry()
	}

	if b.City != newBackend.GetCity() {
		log.Infof("[%s] backend %s updated, city changed from %s to %s", b.Fqdn, b.Address, b.City, newBackend.GetCity())
		b.City = newBackend.GetCity()
	}

	if b.ASN != newBackend.GetASN() {
		log.Infof("[%s] backend %s updated, asn changed from %s to %s", b.Fqdn, b.Address, b.ASN, newBackend.GetASN())
		b.ASN = newBackend.GetASN()
	}

	if b.Location != newBackend.GetLocation() {
		log.Infof("[%s] backend %s updated, location changed from %s to %s", b.Fqdn, b.Address, b.Location, newBackend.GetLocation())
		b.Location = newBackend.GetLocation()
	}

	if b.CoordinatesSet != newBackend.HasCoordinates() || b.Latitude != newBackend.GetLatitude() || b.Longitude != newBackend.GetLongitude() {
		log.Infof("[%s] backend %s updated, coordinates changed", b.Fqdn, b.Address)
		b.CoordinatesSet = newBackend.HasCoordinates()
		b.Latitude = newBackend.GetLatitude()
		b.Longitude = newBackend.GetLongitude()
	}

	// Compare tags slice
	if !tagsEqual(b.Tags, newBackend.GetTags()) {
		log.Infof("[%s] backend %s updated, tags changed", b.Fqdn, b.Address)
		b.Tags = newBackend.GetTags()
	}

	// Check if health checks have changed
	if !healthChecksEqual(b.HealthChecks, newBackend.GetHealthChecks()) {
		log.Infof("[%s] backend %s health checks have changed.", b.Fqdn, b.Address)
		b.HealthChecks = newBackend.GetHealthChecks()
	}
}

func (b *Backend) runHealthChecks(maxRetries int, scrapeTimeout time.Duration) {
	start := time.Now()
	b.mutex.Lock()
	b.LastHealthcheck = start
	b.mutex.Unlock()
	var wg sync.WaitGroup
	results := make([]bool, len(b.HealthChecks))

	log.Debugf("[%s] starting health check for backend: %s", b.Fqdn, b.Address)

	// Gather the list of health check types
	var healthChecksList []string
	for _, healthCheck := range b.HealthChecks {
		healthChecksList = append(healthChecksList, healthCheck.GetType())
	}

	// Iterate over all health checks
	for i, hc := range b.HealthChecks {
		wg.Add(1) // Increment WaitGroup counter for each health check
		go func(i int, hc GenericHealthCheck) {
			defer wg.Done() // Decrement WaitGroup counter when the goroutine finishes

			// Create a context with timeout for the health check
			ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
			defer cancel()

			resultChan := make(chan bool, 1)

			// Goroutine to perform the health check
			go func() {
				resultChan <- hc.PerformCheck(b, b.Fqdn, maxRetries)
			}()

			// Wait for either the result or a timeout
			select {
			case results[i] = <-resultChan:
			case <-ctx.Done():
				log.Debugf("[%s] health check timed out for backend: %s, check: %s", b.Fqdn, b.Address, hc.GetType())
				results[i] = false
			}
		}(i, hc)
	}

	// Wait for all health check goroutines to complete before returning the results.
	wg.Wait()

	// Record wall-clock duration of this health check run for fastest-mode routing.
	elapsed := time.Since(start)

	// Store old alive state for comparision
	oldAlive := b.Alive

	// Update the backend's Alive status
	alive := true
	for _, result := range results {
		if !result {
			alive = false
			break
		}
	}
	b.mutex.Lock()
	b.Alive = alive
	b.ResponseTime = elapsed
	b.mutex.Unlock()

	// Log backend health changes with higher log level
	if b.Alive != oldAlive {
		log.Infof("[%s] backend status change [address=%s]: alive changed from %v to %v", b.Fqdn, b.Address, oldAlive, b.Alive)
	}

	// Keep old log format for log parsing
	log.Debugf("[%s] backend status [address=%s]: healthchecks=%s alive=%v", b.Fqdn, b.Address, healthChecksList, b.Alive)
}

func (b *Backend) IsHealthy() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.Alive && b.Enable
}

// tagsEqual compares two slices of strings (tags) for equality.
func tagsEqual(t1, t2 []string) bool {
	if len(t1) != len(t2) {
		return false
	}
	for i, tag := range t1 {
		if tag != t2[i] {
			return false
		}
	}
	return true
}

type BackendInterface interface {
	GetFqdn() string
	SetFqdn(fqdn string)
	GetDescription() string
	GetAddress() string
	GetPriority() int
	GetWeight() int
	IsEnabled() bool
	GetTags() []string
	GetHealthChecks() []GenericHealthCheck
	GetTimeout() string
	GetCountry() string
	GetCity() string
	GetASN() string
	GetLocation() string
	GetLatitude() float64
	GetLongitude() float64
	HasCoordinates() bool
	GetResponseTime() time.Duration
	IsHealthy() bool
	runHealthChecks(retries int, timeout time.Duration)
	removeBackend()
	updateBackend(newBackend BackendInterface)
	Lock()
	Unlock()
}
