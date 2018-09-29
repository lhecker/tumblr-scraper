package semaphore

type Semaphore struct {
	waiters chan struct{}
}

func NewSemaphore(capacity int) *Semaphore {
	if capacity <= 0 {
		panic("invalid capacity")
	}
	return &Semaphore{
		waiters: make(chan struct{}, capacity),
	}
}

func (s Semaphore) Close() {
	close(s.waiters)
}

func (s Semaphore) Acquire() {
	s.waiters <- struct{}{}
}

func (s Semaphore) Release() {
	<-s.waiters
}
