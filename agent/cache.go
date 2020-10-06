// Copyright 2014 The fleet Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"encoding/json"
	"sync"

	"github.com/cea-hpc/fleet/job"
)

type agentCache struct {
	jobs map[string]job.JobState
	mu   *sync.RWMutex
}

func newAgentCache() *agentCache {
	return &agentCache{
		jobs: map[string]job.JobState{},
		mu:   new(sync.RWMutex),
	}
}

func (ac *agentCache) MarshalJSON() ([]byte, error) {
	type ds struct {
		TargetStates map[string]job.JobState
	}
	data := ds{
		TargetStates: map[string]job.JobState(ac.jobs),
	}
	return json.Marshal(data)
}

func (ac *agentCache) setTargetState(jobName string, state job.JobState) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.jobs[jobName] = state
}

func (ac *agentCache) dropTargetState(jobName string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	delete(ac.jobs, jobName)
}

func (ac *agentCache) launchedJobs() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	jobs := make([]string, 0)
	for j, ts := range ac.jobs {
		if ts == job.JobStateLaunched {
			jobs = append(jobs, j)
		}
	}
	return jobs
}

func (ac *agentCache) loadedJobs() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	jobs := make([]string, 0)
	for j, ts := range ac.jobs {
		if ts == job.JobStateLoaded {
			jobs = append(jobs, j)
		}
	}
	return jobs
}
