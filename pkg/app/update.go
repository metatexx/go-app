package app

// updateManager manages scheduled updates for UI components. It ensures that
// components are updated in an order respecting their depth in the UI
// hierarchy.
type updateManager struct {
	pending []map[Composer]int
}

// Add queues the given component for an update.
func (m *updateManager) Add(v Composer) {
	depth := int(v.depth())
	if len(m.pending) <= depth {
		size := max(depth+1, 100)
		pending := make([]map[Composer]int, size)
		copy(pending, m.pending)
		m.pending = pending
	}

	updates := m.pending[depth]
	if updates == nil {
		updates = make(map[Composer]int)
		m.pending[depth] = updates
	}
	updates[v]++
}

// Done removes the given Composer from the update queue, marking it as updated.
func (m *updateManager) Done(v Composer) {
	depth := v.depth()
	if len(m.pending) <= int(depth) {
		return
	}

	updates := m.pending[depth]
	if updates == nil {
		return
	}
	if updates[v]--; updates[v] < 1 {
		delete(updates, v)
	}
}

// ForEach iterates over all queued components, invoking the provided function
// on each.
func (m *updateManager) ForEach(do func(Composer)) {
	for _, updates := range m.pending {
		for compo := range updates {
			do(compo)
		}
	}
}
