package jobutil

import "sync"

func Parallel[T any](workers int, jobs []T, fn func(T)) {
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j T) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(j)
		}(job)
	}
	wg.Wait()
}
