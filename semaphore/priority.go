package semaphore

import (
	"container/heap"
	"sync"
)

type PrioritySemaphore struct {
	lock      sync.Mutex
	waiters   queue
	capacity  int
	allocated int
}

func NewPrioritySemaphore(capacity int) *PrioritySemaphore {
	if capacity <= 0 {
		panic("invalid capacity")
	}

	s := &PrioritySemaphore{
		capacity: capacity,
		waiters:  make(queue, 0),
	}
	heap.Init(&s.waiters)
	return s
}

func (s *PrioritySemaphore) Acquire(priority int) {
	s.lock.Lock()

	if s.allocated < s.capacity {
		s.allocated++
		s.lock.Unlock()
		return
	}

	ch := make(chan struct{})
	heap.Push(&s.waiters, queueEntry{ch, priority})

	s.lock.Unlock()

	<-ch
}

func (s *PrioritySemaphore) Release() {
	s.lock.Lock()

	s.allocated--

	for s.allocated < s.capacity && s.waiters.Len() != 0 {
		e := heap.Pop(&s.waiters).(queueEntry)
		close(e.ch)
		s.allocated++
	}

	s.lock.Unlock()
}

func (s *PrioritySemaphore) Do(priority int, f func() error) error {
	s.Acquire(priority)
	defer s.Release()
	return f()
}

type queueEntry struct {
	ch       chan struct{}
	priority int
}

type queue []queueEntry

func (pq queue) Len() int {
	return len(pq)
}

func (pq queue) Less(i, j int) bool {
	return pq[i].priority > pq[j].priority
}

func (pq queue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *queue) Push(x interface{}) {
	*pq = append(*pq, x.(queueEntry))
}

func (pq *queue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}
