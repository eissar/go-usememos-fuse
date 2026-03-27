package main

import (
	"sync"
	"testing"
)

func TestInodeManager_Allocate(t *testing.T) {
	m := NewInodeManager(2)

	// Allocate first inode
	ino1 := m.Allocate()
	if ino1 != 2 {
		t.Errorf("Expected first inode 2, got %d", ino1)
	}

	// Allocate second inode
	ino2 := m.Allocate()
	if ino2 != 3 {
		t.Errorf("Expected second inode 3, got %d", ino2)
	}

	// Count should be 2
	if m.Count() != 2 {
		t.Errorf("Expected count 2, got %d", m.Count())
	}

	// Both should be in use
	if !m.IsInUse(ino1) {
		t.Error("Expected ino1 to be in use")
	}
	if !m.IsInUse(ino2) {
		t.Error("Expected ino2 to be in use")
	}
}

func TestInodeManager_AllocateFrom(t *testing.T) {
	m := NewInodeManager(2)

	// Reserve inode 5
	ino, ok := m.AllocateFrom(5)
	if !ok || ino != 5 {
		t.Error("Expected to allocate inode 5")
	}

	// Try to reserve same inode again
	_, ok = m.AllocateFrom(5)
	if ok {
		t.Error("Expected allocation to fail (duplicate)")
	}

	// Allocate next should be 6 (not 2, since 5 was reserved)
	next := m.Allocate()
	if next != 6 {
		t.Errorf("Expected next inode 6, got %d", next)
	}
}

func TestInodeManager_Release(t *testing.T) {
	m := NewInodeManager(2)

	ino := m.Allocate()
	if !m.IsInUse(ino) {
		t.Error("Expected inode to be in use")
	}

	m.Release(ino)
	if m.IsInUse(ino) {
		t.Error("Expected inode to be released")
	}

	if m.Count() != 0 {
		t.Errorf("Expected count 0 after release, got %d", m.Count())
	}
}

func TestInodeManager_Concurrent(t *testing.T) {
	m := NewInodeManager(2)
	var wg sync.WaitGroup
	count := 100

	// Concurrent allocations
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Allocate()
		}()
	}
	wg.Wait()

	if m.Count() != count {
		t.Errorf("Expected %d inodes, got %d", count, m.Count())
	}
}
