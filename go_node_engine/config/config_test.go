package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func useTempConfig(t *testing.T) {
	t.Helper()

	originalDir, originalPath := confDir, confPath
	confDir = t.TempDir()
	confPath = filepath.Join(confDir, "conf.json")
	t.Cleanup(func() {
		confDir, confPath = originalDir, originalPath
	})
}

func TestConcurrentReadCreatesDefaultConfig(t *testing.T) {
	useTempConfig(t)

	const readers = 32
	start := make(chan struct{})
	errs := make(chan error, readers)
	var wg sync.WaitGroup

	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := Read()
			if err != nil {
				errs <- err
				return
			}
			if !reflect.DeepEqual(got, Default()) {
				errs <- fmt.Errorf("Read() = %#v, want default configuration", got)
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("reading persisted config: %v", err)
	}
	var persisted ConfFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("persisted config is invalid JSON: %v", err)
	}
	if !reflect.DeepEqual(persisted, Default()) {
		t.Fatalf("persisted config = %#v, want default configuration", persisted)
	}
}

func TestConcurrentReadRepairsInvalidConfigOnce(t *testing.T) {
	useTempConfig(t)

	if err := os.WriteFile(confPath, []byte("not-json"), 0644); err != nil {
		t.Fatalf("writing invalid config: %v", err)
	}

	const readers = 32
	type result struct {
		conf ConfFile
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, readers)
	var wg sync.WaitGroup

	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			conf, err := Read()
			results <- result{conf: conf, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	repairErrors := 0
	for result := range results {
		if result.err != nil {
			repairErrors++
			continue
		}
		if !reflect.DeepEqual(result.conf, Default()) {
			t.Errorf("Read() = %#v after repair, want default configuration", result.conf)
		}
	}
	if repairErrors != 1 {
		t.Errorf("repair errors = %d, want exactly one", repairErrors)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	useTempConfig(t)

	if err := Write(Default()); err != nil {
		t.Fatalf("writing initial config: %v", err)
	}

	const (
		writers       = 8
		readers       = 16
		operations    = 100
		totalRoutines = writers + readers
	)
	start := make(chan struct{})
	errs := make(chan error, totalRoutines)
	var wg sync.WaitGroup

	for writer := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for operation := range operations {
				cfg := Default()
				cfg.ClusterAddress = fmt.Sprintf("writer-%d-operation-%d", writer, operation)
				if err := Write(cfg); err != nil {
					errs <- fmt.Errorf("Write(): %w", err)
					return
				}
			}
		}()
	}

	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range operations {
				cfg, err := Read()
				if err != nil {
					errs <- fmt.Errorf("Read(): %w", err)
					return
				}
				if cfg.ConfVersion != Default().ConfVersion {
					errs <- fmt.Errorf("Read() returned incomplete config: %#v", cfg)
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("reading final config: %v", err)
	}
	var persisted ConfFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("final config is invalid JSON: %v", err)
	}
}

func TestVirtualizationReadsLegacyKeys(t *testing.T) {
	var v Virtualization
	legacy := `{"virutalizaiton_name":"containerd","virutalizaiton_runtime":"docker","virutalizaiton_active":true,"virutalizaiton_config":[]}`
	if err := json.Unmarshal([]byte(legacy), &v); err != nil {
		t.Fatal(err)
	}
	if v.Name != "containerd" || v.Runtime != "docker" || !v.Active {
		t.Fatalf("legacy keys not read: %+v", v)
	}

	// Current keys win when both are present.
	both := `{"virtualization_runtime":"crosvm","virtualization_active":true,"virutalizaiton_runtime":"docker"}`
	if err := json.Unmarshal([]byte(both), &v); err != nil {
		t.Fatal(err)
	}
	if v.Runtime != "crosvm" {
		t.Fatalf("current keys must take precedence: %+v", v)
	}
}

// A config emptied by an incompatible NodeEngine (entries whose keys it could not
// read) gets its container runtime back, and the repair is persisted.
func TestReadRepairsEmptiedVirtualizations(t *testing.T) {
	useTempConfig(t)
	emptied := `{"conf_version":"1.0","virtualizations":[
		{"virutalizaiton_name":"","virutalizaiton_runtime":"","virutalizaiton_active":false,"virutalizaiton_config":null},
		{"virutalizaiton_name":"","virutalizaiton_runtime":"","virutalizaiton_active":false,"virutalizaiton_config":null}]}`
	if err := os.WriteFile(confPath, []byte(emptied), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Virtualizations, Default().Virtualizations) {
		t.Fatalf("virtualizations = %+v; want the default containerd entry", got.Virtualizations)
	}
	data, _ := os.ReadFile(confPath)
	if !strings.Contains(string(data), `"virtualization_runtime":"docker"`) {
		t.Fatalf("repair not persisted:\n%s", data)
	}
}

// Valid entries are left alone, including a deliberately disabled containerd.
func TestReadKeepsValidVirtualizations(t *testing.T) {
	useTempConfig(t)
	conf := Default()
	conf.Virtualizations[0].Active = false
	conf.Virtualizations = append(conf.Virtualizations, Virtualization{Name: "crosvm", Runtime: string(CROSVM_RUNTIME), Active: true, Config: []string{}})
	if err := Write(conf); err != nil {
		t.Fatal(err)
	}
	got, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Virtualizations, conf.Virtualizations) {
		t.Fatalf("got %+v; want %+v", got.Virtualizations, conf.Virtualizations)
	}
}
