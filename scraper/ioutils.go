package scraper

import (
	"sync"
)

var (
	tempFilesLock sync.Mutex
	tempFiles     = make(map[string]struct{})
)

func acquireTempFile(path string) bool {
	tempFilesLock.Lock()
	defer tempFilesLock.Unlock()

	if _, ok := tempFiles[path]; ok {
		return false
	}

	tempFiles[path] = struct{}{}
	return true
}

func releaseTempFile(path string) {
	tempFilesLock.Lock()
	defer tempFilesLock.Unlock()

	delete(tempFiles, path)
}
