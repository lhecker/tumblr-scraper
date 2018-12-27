package scraper

import (
	"sync"
)

var (
	lockedFilesMutex sync.Mutex
	lockedFiles      = make(map[string]struct{})
)

func acquireFile(path string) bool {
	lockedFilesMutex.Lock()
	defer lockedFilesMutex.Unlock()

	if _, ok := lockedFiles[path]; ok {
		return false
	}

	lockedFiles[path] = struct{}{}
	return true
}

func releaseFile(path string) {
	lockedFilesMutex.Lock()
	defer lockedFilesMutex.Unlock()

	delete(lockedFiles, path)
}
