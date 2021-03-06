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

package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/go-systemd/activation"
	"golang.org/x/net/context"

	"github.com/cea-hpc/fleet/agent"
	"github.com/cea-hpc/fleet/api"
	"github.com/cea-hpc/fleet/config"
	"github.com/cea-hpc/fleet/engine"
	"github.com/cea-hpc/fleet/heart"
	"github.com/cea-hpc/fleet/log"
	"github.com/cea-hpc/fleet/machine"
	"github.com/cea-hpc/fleet/pkg"
	"github.com/cea-hpc/fleet/pkg/lease"
	"github.com/cea-hpc/fleet/registry"
	"github.com/cea-hpc/fleet/registry/rpc"
	"github.com/cea-hpc/fleet/systemd"
	"github.com/cea-hpc/fleet/unit"
	"github.com/cea-hpc/fleet/version"
)

const (
	// machineStateRefreshInterval is the amount of time the server will
	// wait before each attempt to refresh the local machine state
	machineStateRefreshInterval = time.Minute

	shutdownTimeout = time.Minute
)

type Server struct {
	agent          *agent.Agent
	aReconciler    *agent.AgentReconciler
	usPub          *agent.UnitStatePublisher
	usGen          *unit.UnitStateGenerator
	engine         *engine.Engine
	mach           *machine.CoreOSMachine
	hrt            heart.Heart
	mon            *Monitor
	api            *api.Server
	disableEngine  bool
	reconfigServer bool
	restartServer  bool
	eClient        etcd.Client

	engineReconcileInterval time.Duration

	killc chan struct{}  // used to signal monitor to shutdown server
	stopc chan struct{}  // used to terminate all other goroutines
	wg    sync.WaitGroup // used to co-ordinate shutdown
}

func New(cfg config.Config, listeners []net.Listener) (*Server, error) {
	agentTTL, err := time.ParseDuration(cfg.AgentTTL)
	if err != nil {
		return nil, err
	}

	mgr, err := systemd.NewSystemdUnitManager(cfg.UnitsDirectory, cfg.SystemdUser)
	if err != nil {
		return nil, err
	}

	mach, err := newMachineFromConfig(cfg, mgr)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := pkg.ReadTLSConfigFiles(cfg.EtcdCAFile, cfg.EtcdCertFile, cfg.EtcdKeyFile)
	if err != nil {
		return nil, err
	}

	eCfg := etcd.Config{
		Transport:               &http.Transport{TLSClientConfig: tlsConfig},
		Endpoints:               cfg.EtcdServers,
		HeaderTimeoutPerRequest: (time.Duration(cfg.EtcdRequestTimeout*1000) * time.Millisecond),
		Username:                cfg.EtcdUsername,
		Password:                cfg.EtcdPassword,
	}

	eClient, err := etcd.New(eCfg)
	if err != nil {
		return nil, err
	}

	kAPI := etcd.NewKeysAPI(eClient)

	var (
		reg        engine.CompleteRegistry
		genericReg interface{}
	)
	lManager := lease.NewEtcdLeaseManager(kAPI, cfg.EtcdKeyPrefix)

	if !cfg.EnableGRPC {
		genericReg = registry.NewEtcdRegistry(kAPI, cfg.EtcdKeyPrefix)
		if obj, ok := genericReg.(engine.CompleteRegistry); ok {
			reg = obj
		}
	} else {
		etcdReg := registry.NewEtcdRegistry(kAPI, cfg.EtcdKeyPrefix)
		genericReg = rpc.NewRegistryMux(etcdReg, mach, lManager)
		if obj, ok := genericReg.(engine.CompleteRegistry); ok {
			reg = obj
		}
	}

	pub := agent.NewUnitStatePublisher(reg, mach, agentTTL)
	gen := unit.NewUnitStateGenerator(mgr)

	a := agent.New(mgr, gen, reg, mach, agentTTL)

	var rStream pkg.EventStream
	if !cfg.DisableWatches {
		rStream = registry.NewEtcdEventStream(kAPI, cfg.EtcdKeyPrefix)
	}

	ar := agent.NewReconciler(reg, rStream)

	var e *engine.Engine
	if !cfg.EnableGRPC {
		e = engine.New(reg, lManager, rStream, mach, nil)
	} else {
		regMux := genericReg.(*rpc.RegistryMux)
		e = engine.New(reg, lManager, rStream, mach, regMux.EngineChanged)
		if cfg.DisableEngine {
			go regMux.ConnectToRegistry(e)
		}
	}

	if len(listeners) == 0 {
		listeners, err = activation.Listeners(false)
		if err != nil {
			return nil, err
		}
	}

	hrt := heart.New(reg, mach)
	mon := NewMonitor(agentTTL)

	apiServer := api.NewServer(listeners, api.NewServeMux(reg, cfg.TokenLimit))
	apiServer.Serve()

	eIval := time.Duration(cfg.EngineReconcileInterval*1000) * time.Millisecond

	srv := Server{
		agent:                   a,
		aReconciler:             ar,
		usGen:                   gen,
		usPub:                   pub,
		engine:                  e,
		mach:                    mach,
		hrt:                     hrt,
		mon:                     mon,
		api:                     apiServer,
		killc:                   make(chan struct{}),
		stopc:                   nil,
		engineReconcileInterval: eIval,
		disableEngine:           cfg.DisableEngine,
		reconfigServer:          false,
		restartServer:           false,
		eClient:                 eClient,
	}

	return &srv, nil
}

func newMachineFromConfig(cfg config.Config, mgr unit.UnitManager) (*machine.CoreOSMachine, error) {
	state := machine.MachineState{
		PublicIP:     cfg.PublicIP,
		Metadata:     cfg.Metadata(),
		Capabilities: cfg.Capabilities(),
		Version:      version.Version,
	}

	mach := machine.NewCoreOSMachine(state, mgr)
	mach.Refresh()

	if mach.State().ID == "" {
		return nil, errors.New("unable to determine local machine ID")
	}

	return mach, nil
}

func (s *Server) Run() {
	log.Infof("Establishing etcd connectivity")

	var err error
	for sleep := time.Second; ; sleep = pkg.ExpBackoff(sleep, time.Minute) {
		if s.restartServer {
			_, err = s.hrt.Beat(s.mon.TTL)
			if err == nil {
				log.Infof("hrt.Beat() success")
				break
			}
		} else {
			_, err = s.hrt.Register(s.mon.TTL)
			if err == nil {
				log.Infof("hrt.Register() success")
				break
			}
		}
		log.Warningf("Server register machine failed: %v, retrying in %.0f sec.", err, sleep.Seconds())
		log.Infof("Syncing etcd client")
		s.eClient.Sync(context.Background())
		time.Sleep(sleep)
	}

	go s.Supervise()

	log.Infof("Starting server components")
	s.stopc = make(chan struct{})
	s.wg = sync.WaitGroup{}
	beatc := make(chan *unit.UnitStateHeartbeat)

	components := []func(){
		func() { s.api.Available(s.stopc) },
		func() { s.mach.PeriodicRefresh(machineStateRefreshInterval, s.stopc) },
		func() { s.agent.Heartbeat(s.stopc) },
		func() { s.aReconciler.Run(s.agent, s.stopc) },
		func() { s.usGen.Run(beatc, s.stopc) },
		func() { s.usPub.Run(beatc, s.stopc) },
	}
	if s.disableEngine {
		log.Info("Not starting engine; disable-engine is set")
	} else {
		components = append(components, func() { s.engine.Run(s.engineReconcileInterval, s.stopc) })
	}
	for _, f := range components {
		f := f
		s.wg.Add(1)
		go func() {
			f()
			s.wg.Done()
		}()
	}
}

// Supervise monitors the life of the Server and coordinates its shutdown.
// A shutdown occurs when the monitor returns, either because a health check
// fails or a user triggers a shutdown. If the shutdown is due to a health
// check failure, the Server is restarted. Supervise will block shutdown until
// all components have finished shutting down or a timeout occurs; if this
// happens, the Server will not automatically be restarted.
func (s *Server) Supervise() {
	sd, err := s.mon.Monitor(s.hrt, s.killc)
	if sd {
		log.Infof("Server monitor triggered: told to shut down")
	} else {
		log.Errorf("Server monitor triggered: %v", err)
	}
	close(s.stopc)
	if !sd {
		log.Infof("Health check failure : purging local units")
		s.aReconciler.Purge(s.agent)
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		log.Errorf("Timed out waiting for server to shut down. Panicking the server without cleanup.")
		panic("Failed server shutdown. Panic")
	}
	if !sd {
		log.Infof("Restarting server")
		s.SetRestartServer(true)
		s.Run()
		s.SetRestartServer(false)
	}
}

// Kill is used to gracefully terminate the server by triggering the Monitor to shut down
func (s *Server) Kill() {
	if !s.reconfigServer {
		close(s.killc)
	}
}

func (s *Server) Purge() {
	s.aReconciler.Purge(s.agent)
	s.usPub.Purge()
	s.engine.Purge()
	s.hrt.Clear()
}

func (s *Server) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Agent              *agent.Agent
		UnitStatePublisher *agent.UnitStatePublisher
		UnitStateGenerator *unit.UnitStateGenerator
	}{
		Agent:              s.agent,
		UnitStatePublisher: s.usPub,
		UnitStateGenerator: s.usGen,
	})
}

func (s *Server) GetApiServerListeners() []net.Listener {
	return s.api.GetListeners()
}

func (s *Server) SetReconfigServer(isReconfigServer bool) {
	s.reconfigServer = isReconfigServer
}

func (s *Server) SetRestartServer(isRestartServer bool) {
	s.restartServer = isRestartServer
}
