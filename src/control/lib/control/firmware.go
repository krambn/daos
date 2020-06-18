//
// (C) Copyright 2020 Intel Corporation.
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
// +build firmware

package control

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	ctlpb "github.com/daos-stack/daos/src/control/common/proto/ctl"
	"github.com/daos-stack/daos/src/control/server/storage"
)

type (
	// FirmwareQueryReq is a request for firmware information for storage
	// devices.
	FirmwareQueryReq struct {
		unaryRequest
		SCM  bool // Query SCM devices
		NVMe bool // Query NVMe devices
	}

	// FirmwareQueryResp returns storage device firmware information.
	FirmwareQueryResp struct {
		HostErrorsResp
		HostSCMFirmware map[string][]*SCMFirmwareQueryResult
	}

	// SCMFirmwareQueryResult represents the results of a firmware query
	// for a single SCM device.
	SCMFirmwareQueryResult struct {
		DeviceUID    string
		DeviceHandle uint32
		Info         storage.ScmFirmwareInfo
		Error        error
	}
)

// addHostResponse is responsible for validating the given HostResponse
// and adding it to the FirmwareQueryResp.
func (ssp *FirmwareQueryResp) addHostResponse(hr *HostResponse) error {
	pbResp, ok := hr.Message.(*ctlpb.FirmwareQueryResp)
	if !ok {
		return errors.Errorf("unable to unpack message: %+v", hr.Message)
	}

	scmResults := make([]*SCMFirmwareQueryResult, 0, len(pbResp.ScmResults))

	for _, pbScmRes := range pbResp.ScmResults {
		scmResults = append(scmResults, &SCMFirmwareQueryResult{
			DeviceUID:    pbScmRes.Uid,
			DeviceHandle: pbScmRes.Handle,
		})
	}
	return nil
}

// FirmwareQuery concurrently requests device firmware information from
// all hosts supplied in the request's hostlist, or all configured hosts
// if not explicitly specified. The function blocks until all results
// (successful or otherwise) are received, and returns a single response
// structure containing results for all host firmware query operations.
func FirmwareQuery(ctx context.Context, rpcClient UnaryInvoker, req *FirmwareQueryReq) (*FirmwareQueryResp, error) {
	req.setRPC(func(ctx context.Context, conn *grpc.ClientConn) (proto.Message, error) {
		return ctlpb.NewMgmtCtlClient(conn).FirmwareQuery(ctx, &ctlpb.FirmwareQueryReq{
			QueryScm:  req.SCM,
			QueryNvme: req.NVMe,
		})
	})

	unaryResp, err := rpcClient.InvokeUnaryRPC(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := new(FirmwareQueryResp)
	for _, hostResp := range unaryResp.Responses {
		if hostResp.Error != nil {
			if err := resp.addHostError(hostResp.Addr, hostResp.Error); err != nil {
				return nil, err
			}
			continue
		}

		if err := resp.addHostResponse(hostResp); err != nil {
			return nil, err
		}
	}

	return resp, nil
}
