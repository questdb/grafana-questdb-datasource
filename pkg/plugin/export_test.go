package plugin

// EnableBindParams enables prepared statements on h (the default a real Connect
// applies), standing in for Connect in tests that exercise MutateQuery directly.
func EnableBindParams(h *QuestDB) {
	h.bindParamsEnabled.Store(true)
}
