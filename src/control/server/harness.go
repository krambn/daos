//
// (C) Copyright 2019-2020 Intel Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// GOVERNMENT LICENSE RIGHTS-OPEN SOURCE SOFTWARE
// The Government's rights to use, modify, reproduce, release, perform, display,
// or disclose this software are subject to the terms of the Apache License as
// provided in Contract No. 8F-30005.
// Any reproduction of computer software, computer software documentation, or
// portions thereof marked with this legend must also reproduce the markings.
//

package server

import (
	"context"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/daos-stack/daos/src/control/lib/atm"
	"github.com/daos-stack/daos/src/control/logging"
	"github.com/daos-stack/daos/src/control/system"
)

const (
	defaultRequestTimeout = 3 * time.Second
	defaultStartTimeout   = 10 * defaultRequestTimeout
)

// IOServerHarness is responsible for managing IOServer instances.
type IOServerHarness struct {
	sync.RWMutex
	log              logging.Logger
	instances        []*IOServerInstance
	started          atm.Bool
	rankReqTimeout   time.Duration
	rankStartTimeout time.Duration
}

// NewIOServerHarness returns an initialized *IOServerHarness.
func NewIOServerHarness(log logging.Logger) *IOServerHarness {
	return &IOServerHarness{
		log:              log,
		instances:        make([]*IOServerInstance, 0, maxIOServers),
		started:          atm.NewBool(false),
		rankReqTimeout:   defaultRequestTimeout,
		rankStartTimeout: defaultStartTimeout,
	}
}

// isStarted indicates whether the IOServerHarness is in a running state.
func (h *IOServerHarness) isStarted() bool {
	return h.started.Load()
}

// Instances safely returns harness' IOServerInstances.
func (h *IOServerHarness) Instances() []*IOServerInstance {
	h.RLock()
	defer h.RUnlock()
	return h.instances
}

// FilterInstancesByRank safely returns harness' IOServerInstances that match any
// of a list of ranks.
func (h *IOServerHarness) FilterInstancesByRank(ranks []system.Rank) ([]*IOServerInstance, error) {
	h.RLock()
	defer h.RUnlock()

	out := make([]*IOServerInstance, 0, maxIOServers)

	for _, i := range h.instances {
		r, err := i.GetRank()
		if err != nil {
			return nil, errors.WithMessage(err, "filtering instances by rank")
		}
		if r.InList(ranks) {
			out = append(out, i)
		}
	}

	return out, nil
}

// AddInstance adds a new IOServer instance to be managed.
func (h *IOServerHarness) AddInstance(srv *IOServerInstance) error {
	if h.isStarted() {
		return errors.New("can't add instance to already-started harness")
	}

	h.Lock()
	defer h.Unlock()
	srv.setIndex(uint32(len(h.instances)))

	h.instances = append(h.instances, srv)
	return nil
}

// GetMSLeaderInstance returns a managed IO Server instance to be used as a
// management target and fails if selected instance is not MS Leader.
func (h *IOServerHarness) GetMSLeaderInstance() (*IOServerInstance, error) {
	if !h.isStarted() {
		return nil, FaultHarnessNotStarted
	}

	h.RLock()
	defer h.RUnlock()

	if len(h.instances) == 0 {
		return nil, errors.New("harness has no managed instances")
	}

	var err error
	for _, mi := range h.instances {
		// try each instance, returning the first one that is a replica (if any are)
		if err = checkIsMSReplica(mi); err == nil {
			return mi, nil
		}
	}

	return nil, err
}

// Start starts harness by setting up and starting dRPC before initiating
// configured instances' processing loops.
//
// Run until harness is shutdown.
func (h *IOServerHarness) Start(ctx context.Context, membership *system.Membership, cfg *Configuration) error {
	if h.isStarted() {
		return errors.New("can't start: harness already started")
	}

	// Now we want to block any RPCs that might try to mess with storage
	// (format, firmware update, etc) before attempting to start I/O servers
	// which are using the storage.
	h.started.SetTrue()
	defer h.started.SetFalse()

	if cfg != nil {
		// Single daos_server dRPC server to handle all iosrv requests
		if err := drpcServerSetup(ctx, h.log, cfg.SocketDir, h.Instances(),
			cfg.TransportConfig); err != nil {

			return errors.WithMessage(err, "dRPC server setup")
		}
		defer func() {
			if err := drpcCleanup(cfg.SocketDir); err != nil {
				h.log.Errorf("error during dRPC cleanup: %s", err)
			}
		}()
	}

	for _, srv := range h.Instances() {
		// start first time then relinquish control to instance
		go srv.Run(ctx, membership, cfg)
		srv.startLoop <- true
	}

	<-ctx.Done()
	h.log.Debug("shutting down harness")

	return ctx.Err()
}

type mgmtInfo struct {
	isReplica       bool
	shouldBootstrap bool
}

func getMgmtInfo(srv *IOServerInstance) (*mgmtInfo, error) {
	// Determine if an I/O server needs to createMS or bootstrapMS.
	var err error
	mi := &mgmtInfo{}
	mi.isReplica, mi.shouldBootstrap, err = checkMgmtSvcReplica(
		srv.msClient.cfg.ControlAddr,
		srv.msClient.cfg.AccessPoints,
	)
	if err != nil {
		return nil, err
	}

	return mi, nil
}

// readyRanks returns rank assignment of configured harness instances that are
// in a ready state. Rank assignments can be nil.
func (h *IOServerHarness) readyRanks() []system.Rank {
	h.RLock()
	defer h.RUnlock()

	ranks := make([]system.Rank, 0, maxIOServers)
	for _, srv := range h.instances {
		if srv.hasSuperblock() && srv.isReady() {
			ranks = append(ranks, *srv.getSuperblock().Rank)
		}
	}

	return ranks
}
