package geryon

/*
https://github.com/kubernetes/client-go/blob/ee7a1ba5cdf1292b67a1fdf1fa28f90d2a7b0084/examples/workqueue/main.go#L34
Copyright 2017 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"log"
)

func (g *Geryon) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := g.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer g.queue.Done(key)

	// Invoke the method containing the business logic
	err := g.SyncNamespace(key.(string))

	// Handle the error if something went wrong during the execution of the business logic
	g.handleErr(err, key)
	return true
}

// handleErr checks if an error happened and makes sure we will retry later.
func (g *Geryon) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		g.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if g.queue.NumRequeues(key) < 5 {
		log.Printf("Error syncing %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		g.queue.AddRateLimited(key)
		return
	}

	g.queue.Forget(key)
	log.Printf("Dropping %q out of the queue: %v", key, err)
}

func (g *Geryon) runWorker() {
	for g.processNextItem() {
	}
}
