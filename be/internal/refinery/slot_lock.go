package refinery

import "sync"

// slotLock is a refcounted mutex registered for a (workflowInstanceID,
// nodeID) digest slot. An entry exists in Manager.slots exactly while >=1
// registered autonomous session references that slot, so overlapping
// relaunch-chain sessions always share one mutex identity and nothing is
// left behind after the last StopSession.
type slotLock struct {
	mu   sync.Mutex
	refs int
}

// slotKey builds the map key shared by acquire/release, same shape as the
// former lockSlot key.
func slotKey(workflowInstanceID, nodeID string) string {
	return workflowInstanceID + "/" + nodeID
}

// acquireSlotLock creates-or-gets the slot entry for (workflowInstanceID,
// nodeID), increments its refcount, and returns its mutex. Call once per
// autonomous session started against that slot; pair with releaseSlotLock.
func (m *Manager) acquireSlotLock(workflowInstanceID, nodeID string) *sync.Mutex {
	key := slotKey(workflowInstanceID, nodeID)
	m.slotsMu.Lock()
	defer m.slotsMu.Unlock()
	l, ok := m.slots[key]
	if !ok {
		l = &slotLock{}
		m.slots[key] = l
	}
	l.refs++
	return &l.mu
}

// releaseSlotLock decrements the refcount for (workflowInstanceID, nodeID)
// and drops the entry once no session references it any longer.
func (m *Manager) releaseSlotLock(workflowInstanceID, nodeID string) {
	key := slotKey(workflowInstanceID, nodeID)
	m.slotsMu.Lock()
	defer m.slotsMu.Unlock()
	l, ok := m.slots[key]
	if !ok {
		return
	}
	l.refs--
	if l.refs <= 0 {
		delete(m.slots, key)
	}
}
