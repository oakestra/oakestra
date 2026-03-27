package main

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"syscall"
	"time"
)

// MountRecord represents a single mount that needs tracking and potential cleanup.
type MountRecord struct {
	VolumeID   string    `json:"volume_id"`
	SourcePath string    `json:"source_path"`
	TargetPath string    `json:"target_path"`
	MountedAt  time.Time `json:"mounted_at"`
}

// MountTracker persists mount information to disk and provides cleanup capabilities.
type MountTracker struct {
	statePath string
	mu        sync.Mutex
	mounts    map[string]*MountRecord // key: targetPath
	cleanupCh chan string             // targetPath to cleanup
	stopCh    chan struct{}
	cleanupWg sync.WaitGroup
}

// NewMountTracker creates a new mount tracker that persists to statePath.
func NewMountTracker(statePath string) *MountTracker {
	mt := &MountTracker{
		statePath: statePath,
		mounts:    make(map[string]*MountRecord),
		cleanupCh: make(chan string, 100),
		stopCh:    make(chan struct{}),
	}

	// Load existing state if available
	if err := mt.load(); err != nil {
		log.Printf("[mount-tracker] Failed to load state from %q: %v (starting fresh)", statePath, err)
	} else {
		log.Printf("[mount-tracker] Loaded %d mount records from %q", len(mt.mounts), statePath)
	}

	// Start background cleanup worker
	mt.cleanupWg.Add(1)
	go mt.cleanupWorker()

	return mt
}

// IsMounted checks if a target path is currently tracked as mounted.
func (mt *MountTracker) IsMounted(targetPath string) bool {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	_, exists := mt.mounts[targetPath]
	return exists
}

// AddMount records a new mount and persists it to disk.
func (mt *MountTracker) AddMount(volumeID, sourcePath, targetPath string) error {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	mt.mounts[targetPath] = &MountRecord{
		VolumeID:   volumeID,
		SourcePath: sourcePath,
		TargetPath: targetPath,
		MountedAt:  time.Now(),
	}

	if err := mt.save(); err != nil {
		log.Printf("[mount-tracker] Warning: failed to persist mount record: %v", err)
		// Don't fail the operation - the mount succeeded
	}

	log.Printf("[mount-tracker] Recorded mount: %s → %s", sourcePath, targetPath)
	return nil
}

// RemoveMount attempts to unmount and clean up a target path.
// If unmount fails, it queues the path for background retry.
func (mt *MountTracker) RemoveMount(targetPath string) error {
	mt.mu.Lock()
	_, exists := mt.mounts[targetPath]
	mt.mu.Unlock()

	if !exists {
		log.Printf("[mount-tracker] No record found for %s (already cleaned up?)", targetPath)
		return nil
	}

	// Attempt immediate unmount
	if err := mt.attemptUnmount(targetPath); err != nil {
		log.Printf("[mount-tracker] Immediate unmount failed for %q: %v (queuing for retry)", targetPath, err)
		// Queue for background retry
		select {
		case mt.cleanupCh <- targetPath:
		default:
			log.Printf("[mount-tracker] Warning: cleanup queue full, dropping %s", targetPath)
		}
		return err
	}

	// Success - remove from tracking
	mt.mu.Lock()
	delete(mt.mounts, targetPath)
	mt.mu.Unlock()

	if err := mt.save(); err != nil {
		log.Printf("[mount-tracker] Warning: failed to persist after unmount: %v", err)
	}

	log.Printf("[mount-tracker] Successfully unmounted and removed record: %s", targetPath)
	return nil
}

// attemptUnmount performs the actual unmount and directory removal.
func (mt *MountTracker) attemptUnmount(targetPath string) error {
	// Try unmount
	if err := syscall.Unmount(targetPath, 0); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// Path doesn't exist - consider it already unmounted
	}

	// Remove the directory
	if err := os.RemoveAll(targetPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

// cleanupWorker runs in the background and retries failed unmounts with exponential backoff.
func (mt *MountTracker) cleanupWorker() {
	defer mt.cleanupWg.Done()

	retryQueue := make(map[string]int) // targetPath -> attemptCount
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mt.stopCh:
			log.Printf("[mount-tracker] Cleanup worker stopping...")
			return

		case targetPath := <-mt.cleanupCh:
			retryQueue[targetPath] = 0
			log.Printf("[mount-tracker] Added %s to retry queue", targetPath)

		case <-ticker.C:
			if len(retryQueue) == 0 {
				continue
			}

			log.Printf("[mount-tracker] Processing %d paths in retry queue", len(retryQueue))

			for targetPath, attempts := range retryQueue {
				// Exponential backoff: wait 2^attempts seconds (max 64s)
				waitTime := 1 << attempts
				if waitTime > 64 {
					waitTime = 64
				}
				if attempts > 0 && attempts < 7 {
					// Still in backoff period
					retryQueue[targetPath] = attempts + 1
					continue
				}

				// Attempt unmount
				if err := mt.attemptUnmount(targetPath); err != nil {
					attempts++
					retryQueue[targetPath] = attempts

					if attempts >= 20 {
						log.Printf("[mount-tracker] Giving up on %s after %d attempts (last error: %v)", targetPath, attempts, err)
						delete(retryQueue, targetPath)
						// Leave it in persistent state for manual cleanup or next restart
					} else {
						log.Printf("[mount-tracker] Retry %d failed for %s: %v (will retry)", attempts, targetPath, err)
					}
					continue
				}

				// Success!
				log.Printf("[mount-tracker] Successfully unmounted %s on retry attempt %d", targetPath, attempts+1)
				delete(retryQueue, targetPath)

				// Remove from persistent tracking
				mt.mu.Lock()
				delete(mt.mounts, targetPath)
				mt.mu.Unlock()

				if err := mt.save(); err != nil {
					log.Printf("[mount-tracker] Warning: failed to persist after cleanup: %v", err)
				}
			}
		}
	}
}

// CleanupOrphanedMounts attempts to clean up any mounts that were tracked but not properly unmounted.
// This should be called on startup.
func (mt *MountTracker) CleanupOrphanedMounts() {
	mt.mu.Lock()
	paths := make([]string, 0, len(mt.mounts))
	for targetPath := range mt.mounts {
		paths = append(paths, targetPath)
	}
	mt.mu.Unlock()

	if len(paths) == 0 {
		log.Printf("[mount-tracker] No orphaned mounts to clean up")
		return
	}

	log.Printf("[mount-tracker] Found %d potentially orphaned mounts, attempting cleanup...", len(paths))

	for _, targetPath := range paths {
		if err := mt.attemptUnmount(targetPath); err != nil {
			log.Printf("[mount-tracker] Failed to clean orphaned mount %s: %v (will retry in background)", targetPath, err)
			// Queue for background retry
			select {
			case mt.cleanupCh <- targetPath:
			default:
			}
		} else {
			log.Printf("[mount-tracker] Cleaned up orphaned mount: %s", targetPath)
			mt.mu.Lock()
			delete(mt.mounts, targetPath)
			mt.mu.Unlock()
		}
	}

	// Persist cleaned state
	if err := mt.save(); err != nil {
		log.Printf("[mount-tracker] Warning: failed to persist after orphan cleanup: %v", err)
	}
}

// Close stops the background cleanup worker and persists final state.
func (mt *MountTracker) Close() error {
	close(mt.stopCh)
	mt.cleanupWg.Wait()

	mt.mu.Lock()
	defer mt.mu.Unlock()

	if len(mt.mounts) > 0 {
		log.Printf("[mount-tracker] Shutdown with %d mounts still tracked", len(mt.mounts))
	}

	return mt.save()
}

// load reads the persistent state from disk.
func (mt *MountTracker) load() error {
	data, err := os.ReadFile(mt.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no state file yet - normal for first run
		}
		return err
	}

	var records []*MountRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}

	for _, record := range records {
		mt.mounts[record.TargetPath] = record
	}

	return nil
}

// save writes the current state to disk.
func (mt *MountTracker) save() error {
	records := make([]*MountRecord, 0, len(mt.mounts))
	for _, record := range mt.mounts {
		records = append(records, record)
	}

	data, err := json.Marshal(records)
	if err != nil {
		return err
	}

	// Write atomically via temp file + rename
	tempPath := mt.statePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0640); err != nil {
		return err
	}

	return os.Rename(tempPath, mt.statePath)
}
