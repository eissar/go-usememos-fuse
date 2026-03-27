package main

import (
	"sync"
)

// InodeManager provides thread-safe inode allocation and management.
// Inodes are unique identifiers within the filesystem.
type InodeManager struct {
	mu      sync.RWMutex
	nextIno uint64
	inuse   map[uint64]bool
}

// NewInodeManager creates a new inode manager starting from the given base inode.
// Typically base should be 2 (1 is reserved for root in FUSE).
func NewInodeManager(base uint64) *InodeManager {
	return &InodeManager{
		nextIno: base,
		inuse:   make(map[uint64]bool),
	}
}

// Allocate returns a new unique inode number.
func (m *InodeManager) Allocate() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ino := m.nextIno
	m.nextIno++
	m.inuse[ino] = true
	return ino
}

// AllocateFrom reserves a specific inode number if available.
// Returns the inode and true if successful, 0 and false otherwise.
func (m *InodeManager) AllocateFrom(ino uint64) (uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inuse[ino] {
		return 0, false
	}
	m.inuse[ino] = true
	if ino >= m.nextIno {
		m.nextIno = ino + 1
	}
	return ino, true
}

// Release marks an inode as no longer in use.
func (m *InodeManager) Release(ino uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.inuse, ino)
}

// IsInUse checks if an inode is currently allocated.
func (m *InodeManager) IsInUse(ino uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.inuse[ino]
}

// Count returns the number of allocated inodes.
func (m *InodeManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.inuse)
}
