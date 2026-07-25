package deps

import "sync"

// resetDepmapForTest clears the process-wide depmap cache so tests can
// control what gets loaded. Test-only: the cache is deliberately loaded
// once per process in normal use.
func resetDepmapForTest() {
	depmap = nil
	depmapOnce = sync.Once{}
}
