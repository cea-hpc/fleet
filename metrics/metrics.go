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

package metrics

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type (
	engineFailure string
	registryOp    string
)

const (
	Namespace = "fleet"

	MachineAway     engineFailure = "machine_away"
	RunFailure      engineFailure = "run"
	ScheduleFailure engineFailure = "schedule"
	Get             registryOp    = "get"
	Set             registryOp    = "set"
	GetAll          registryOp    = "get_all"
)

var (
	leaderGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "leader_start_time",
		Help:      "Start time of becoming an engine leader since epoch in seconds.",
	})

	engineTaskCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "task_count_total",
		Help:      "Counter of engine schedule tasks.",
	}, []string{"type"})

	engineTaskFailureCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "task_failure_count_total",
		Help:      "Counter of engine schedule task failures.",
	}, []string{"type"})

	engineReconcileCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "reconcile_count_total",
		Help:      "Counter of reconcile rounds.",
	})

	engineReconcileDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "reconcile_duration_second",
		Help:      "Histogram of time (in seconds) each schedule round takes.",
	})

	engineReconcileFailureCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "reconcile_failure_count_total",
		Help:      "Counter of scheduling failures.",
	}, []string{"type"})

	registryOpCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "registry",
		Name:      "operation_count_total",
		Help:      "Counter of registry operations.",
	}, []string{"type"})

	registryOpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "registry",
		Name:      "operation_duration_second",
		Help:      "Histogram of time (in seconds) each schedule round takes.",
	}, []string{"ops"})

	registryOpFailureCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "registry",
		Name:      "operation_failed_count_total",
		Help:      "Counter of failed registry operations.",
	}, []string{"type"})

	isLeaderGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "is_leader",
		Help:      "Whether I am the cluster leader or not",
	})

	agentsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "agents_available",
		Help:      "Number of available agents.",
	})

	agentLoadGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "agent_load",
		Help:      "Current load on given agent",
	}, []string{"id"})

	clusterJobsGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "engine",
		Name:      "cluster_jobs",
		Help:      "Cluster jobs and whether they are scheduled or not",
	}, []string{"job", "machineid"} )

	agentStateGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "agent",
		Name:      "state",
		Help:      "Agent job states",
	}, []string{"job", "desired_state"} )

	healthyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "agent",
		Name:      "healthy",
		Help:      "Is the agent healthy",
	})

)

func init() {
	prometheus.MustRegister(agentsGauge)
	prometheus.MustRegister(agentStateGauge)
	prometheus.MustRegister(agentLoadGauge)
	prometheus.MustRegister(healthyGauge)
	prometheus.MustRegister(clusterJobsGauge)
	prometheus.MustRegister(isLeaderGauge)
	prometheus.MustRegister(leaderGauge)
	prometheus.MustRegister(engineTaskCount)
	prometheus.MustRegister(engineTaskFailureCount)
	prometheus.MustRegister(engineReconcileCount)
	prometheus.MustRegister(engineReconcileFailureCount)
}

func ReportHealth(healthy bool){
	if healthy {
		healthyGauge.Set(1)
	} else {
		healthyGauge.Set(0)
	}
}

func ResetAgentState() {
	agentStateGauge.Reset()
}

func ReportAgentState(job string, dstate string, nominal bool) {
	agentStateGauge.DeleteLabelValues(job)
	if nominal {
		agentStateGauge.WithLabelValues(job, dstate).Set(1)
	} else {
		agentStateGauge.WithLabelValues(job, dstate).Set(0)
	}
}
func ReportClusterJob(job string, mach_id *string, scheduled bool) {
	agentStateGauge.DeleteLabelValues(job)
	if scheduled {
		clusterJobsGauge.WithLabelValues(job, *mach_id).Set(1)
	} else {
		clusterJobsGauge.WithLabelValues(job, *mach_id).Set(0)
	}
}

func ReportAvailableAgents(nbAgents int) {
	agentsGauge.Set(float64(nbAgents))
}

func ResetAgents() {
	agentLoadGauge.Reset()
}

func ReportAgentLoad(mach_id string, load uint16) {
	agentLoadGauge.WithLabelValues(mach_id).Set(float64(load))
}

func ReportIsEngineLeader() {
	isLeaderGauge.Set(float64(1))
}

func ReportIsNotEngineLeader() {
	isLeaderGauge.Set(float64(0))
}

func ReportEngineLeader() {
	epoch := time.Now().Unix()
	leaderGauge.Add(float64(epoch))
}

func ReportEngineTask(task string) {
	task = strings.ToLower(task)
	engineTaskCount.WithLabelValues(string(task)).Inc()
}
func ReportEngineTaskFailure(task string) {
	task = strings.ToLower(task)
	engineTaskFailureCount.WithLabelValues(string(task)).Inc()
}
func ReportEngineReconcileSuccess(start time.Time) {
	engineReconcileCount.Inc()
	engineReconcileDuration.Observe(float64(time.Since(start)) / float64(time.Second))
}
func ReportEngineReconcileFailure(reason engineFailure) {
	engineReconcileFailureCount.WithLabelValues(string(reason)).Inc()
}
func ReportRegistryOpSuccess(op registryOp, start time.Time) {
	registryOpCount.WithLabelValues(string(op)).Inc()
	registryOpDuration.WithLabelValues(string(op)).Observe(float64(time.Since(start)) / float64(time.Second))
}
func ReportRegistryOpFailure(op registryOp) {
	registryOpFailureCount.WithLabelValues(string(op)).Inc()
}
