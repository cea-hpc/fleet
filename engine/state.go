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

package engine

import (
	"sync"

	"github.com/cea-hpc/fleet/agent"
	"github.com/cea-hpc/fleet/job"
	"github.com/cea-hpc/fleet/machine"
)

type clusterState struct {
	jobs     map[string]*job.Job
	gUnits   map[string]*job.Unit
	machines map[string]*machine.MachineState
	mu       *sync.RWMutex
}

func newClusterState(units []job.Unit, sUnits []job.ScheduledUnit, machines []machine.MachineState) *clusterState {
	sUnitMap := make(map[string]*job.ScheduledUnit)
	for _, sUnit := range sUnits {
		sUnit := sUnit
		sUnitMap[sUnit.Name] = &sUnit
	}

	jMap := make(map[string]*job.Job)
	guMap := make(map[string]*job.Unit)
	for _, u := range units {
		if u.IsGlobal() {
			u := u
			guMap[u.Name] = &u
		} else {
			j := job.Job{
				Name:        u.Name,
				Unit:        u.Unit,
				TargetState: u.TargetState,
			}

			if sUnit, ok := sUnitMap[u.Name]; ok {
				j.TargetMachineID = sUnit.TargetMachineID
				j.State = sUnit.State
			}

			jMap[j.Name] = &j
		}
	}

	mMap := make(map[string]*machine.MachineState, len(machines))
	for _, ms := range machines {
		ms := ms
		mMap[ms.ID] = &ms
	}

	return &clusterState{
		jobs:     jMap,
		gUnits:   guMap,
		machines: mMap,
		mu:       new(sync.RWMutex),
	}
}

func (cs *clusterState) agents() map[string]*agent.AgentState {
	agents := make(map[string]*agent.AgentState, len(cs.machines))
	for _, ms := range cs.machines {
		ms := ms
		agents[ms.ID] = agent.NewAgentState(ms)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, j := range cs.jobs {
		j := j
		if !j.Scheduled() || j.TargetState == job.JobStateInactive {
			continue
		}
		if as, ok := agents[j.TargetMachineID]; ok {
			u := &job.Unit{
				Name:        j.Name,
				Unit:        j.Unit,
				TargetState: j.TargetState,
				Weight:      j.Weight(),
			}
			as.Units[j.Name] = u
		}
	}

	for _, gu := range cs.gUnits {
		gu := gu
		for _, a := range agents {
			if !machine.HasMetadata(a.MState, gu.RequiredTargetMetadata()) {
				continue
			}

			if cExists, _ := a.HasConflict(gu.Name, gu.Conflicts()); cExists {
				continue
			}
			a.Units[gu.Name] = gu
		}
	}

	return agents
}

func (cs *clusterState) schedule(jobName, targetMachineID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	j := cs.jobs[jobName]
	if j == nil {
		return
	}
	j.TargetMachineID = targetMachineID
}

func (cs *clusterState) unschedule(jobName string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	j := cs.jobs[jobName]
	if j == nil {
		return
	}
	j.TargetMachineID = ""
}
