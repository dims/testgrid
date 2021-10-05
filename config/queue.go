/*
Copyright 2021 The TestGrid Authors.

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

package config

import (
	"context"
	"sync"
	"time"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/util"
)

// TestGroupQueue can send test groups to receivers at a specific frequency.
//
// Also contains the ability to modify the next time to send groups.
// First call must be to Init().
// Exported methods are safe to call concurrently.
type TestGroupQueue struct {
	util.Queue
	groups map[string]*configpb.TestGroup
	lock   sync.RWMutex
}

// Init (or reinit) the queue with the specified groups, which should be updated at frequency.
func (q *TestGroupQueue) Init(testGroups []*configpb.TestGroup, when time.Time) {
	n := len(testGroups)
	groups := make(map[string]*configpb.TestGroup, n)
	names := make([]string, n)

	for i, tg := range testGroups {
		name := tg.Name
		names[i] = name
		groups[name] = tg
	}

	q.Queue.Init(names, when)
	q.lock.Lock()
	q.groups = groups
	q.lock.Unlock()
}

// Status of the queue: depth, next item and when the next item is ready.
func (q *TestGroupQueue) Status() (int, *configpb.TestGroup, time.Time) {
	q.lock.RLock()
	defer q.lock.RUnlock()
	var tg *configpb.TestGroup
	var when time.Time
	n, who, when := q.Queue.Status()
	if who != nil {
		tg = q.groups[*who]
	}
	return n, tg, when
}

// Send test groups to receivers until the context expires.
//
// Pops items off the queue when frequency is zero.
// Otherwise reschedules the item after the specified frequency has elapsed.
func (q *TestGroupQueue) Send(ctx context.Context, receivers chan<- *configpb.TestGroup, frequency time.Duration) error {
	ch := make(chan string)
	var err error
	go func() {
		err = q.Queue.Send(ctx, ch, frequency)
		close(ch)
	}()

	for who := range ch {
		q.lock.RLock()
		tg := q.groups[who]
		q.lock.RUnlock()
		if tg == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case receivers <- tg:
		}
	}
	return err
}
