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

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"

	"github.com/daos-stack/daos/src/control/common"
	ctlpb "github.com/daos-stack/daos/src/control/common/proto/ctl"
	"github.com/daos-stack/daos/src/control/lib/control"
	"github.com/daos-stack/daos/src/control/logging"
	"github.com/daos-stack/daos/src/control/system"
)

func TestStorageCommands(t *testing.T) {
	storageFormatReq := &control.StorageFormatReq{Reformat: true}
	storageFormatReq.SetHostList([]string{})

	runCmdTests(t, []cmdTest{
		{
			"Format",
			"storage format",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.StorageFormatReq{}),
			}, " "),
			nil,
		},
		{
			"Format with reformat",
			"storage format --reformat",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.SystemQueryReq{}),
				"ConnectClients",
				printRequest(t, &control.StorageFormatReq{Reformat: true}),
			}, " "),
			nil,
		},
		{
			"Format with invalid ranks filter",
			"storage format --ranks 1-3",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.StorageFormatReq{}),
			}, " "),
			errors.New("--ranks parameter invalid"),
		},
		{
			"Non-system reformat with invalid ranks filter",
			"storage format --reformat --ranks 0-4",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.SystemQueryReq{}),
				"ConnectClients",
				printRequest(t, &control.StorageFormatReq{Reformat: true}),
			}, " "),
			errors.New("--ranks parameter invalid"),
		},
		{
			"Scan",
			"storage scan",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.StorageScanReq{}),
			}, " "),
			nil,
		},
		{
			"Prepare without force",
			"storage prepare",
			"ConnectClients",
			errors.New("consent not given"),
		},
		{
			"Prepare with nvme-only and scm-only",
			"storage prepare --force --nvme-only --scm-only",
			"ConnectClients",
			errors.New("nvme-only and scm-only options should not be set together"),
		},
		{
			"Prepare with scm-only",
			"storage prepare --force --scm-only",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.StoragePrepareReq{
					SCM: &control.ScmPrepareReq{},
				}),
			}, " "),
			nil,
		},
		{
			"Prepare with nvme-only",
			"storage prepare --force --nvme-only",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.StoragePrepareReq{
					NVMe: &control.NvmePrepareReq{},
				}),
			}, " "),
			nil,
		},
		{
			"Prepare with non-existent option",
			"storage prepare --force --nvme",
			"",
			errors.New("unknown flag `nvme'"),
		},
		{
			"Prepare with force and reset",
			"storage prepare --force --reset",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.StoragePrepareReq{
					NVMe: &control.NvmePrepareReq{Reset: true},
					SCM:  &control.ScmPrepareReq{Reset: true},
				}),
			}, " "),
			nil,
		},
		{
			"Prepare with force",
			"storage prepare --force",
			strings.Join([]string{
				"ConnectClients",
				printRequest(t, &control.StoragePrepareReq{
					NVMe: &control.NvmePrepareReq{Reset: false},
					SCM:  &control.ScmPrepareReq{Reset: false},
				}),
			}, " "),
			nil,
		},
		{
			"Set FAULTY device status",
			"storage set nvme-faulty --devuuid abcd",
			"ConnectClients StorageSetFaulty-dev_uuid:\"abcd\" ",
			nil,
		},
		{
			"Set FAULTY device status without device specified",
			"storage set nvme-faulty",
			"ConnectClients StorageSetFaulty",
			errors.New("the required flag `-u, --devuuid' was not specified"),
		},
		{
			"Nonexistent subcommand",
			"storage quack",
			"",
			errors.New("Unknown command"),
		},
	})
}

func TestDmg_Storage_shouldReformatSystem(t *testing.T) {
	for name, tc := range map[string]struct {
		reformat bool
		rankList []system.Rank
		uErr     error
		members  []*ctlpb.SystemMember
		expOK    bool
		expErr   error
	}{
		"no reformat with rank list": {
			rankList: []system.Rank{0, 1},
			expErr:   errors.New("--ranks parameter invalid"),
		},
		"no reformat without rank list": {},
		"empty membership": {
			reformat: true,
		},
		"failed member query": {
			reformat: true,
			uErr:     errors.New("system failed"),
			expErr:   errors.New("system failed"),
		},
		"rank not stopped": {
			reformat: true,
			members: []*ctlpb.SystemMember{
				{Rank: 0, State: uint32(system.MemberStateStopped)},
				{Rank: 1, State: uint32(system.MemberStateJoined)},
			},
			expErr: errors.New("system reformat requires the following 1 rank to be stopped: 1"),
		},
		"ranks not stopped": {
			reformat: true,
			members: []*ctlpb.SystemMember{
				{Rank: 0, State: uint32(system.MemberStateJoined)},
				{Rank: 1, State: uint32(system.MemberStateStopped)},
				{Rank: 5, State: uint32(system.MemberStateJoined)},
				{Rank: 2, State: uint32(system.MemberStateJoined)},
				{Rank: 4, State: uint32(system.MemberStateJoined)},
				{Rank: 3, State: uint32(system.MemberStateJoined)},
				{Rank: 6, State: uint32(system.MemberStateStopped)},
			},
			expErr: errors.New("system reformat requires the following 5 ranks to be stopped: 0,2-5"),
		},
		"system reformat": {
			reformat: true,
			members: []*ctlpb.SystemMember{
				{Rank: 0, State: uint32(system.MemberStateStopped)},
				{Rank: 0, State: uint32(system.MemberStateStopped)},
			},
			expOK: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			log, buf := logging.NewTestLogger(t.Name())
			defer common.ShowBufferOnFailure(t, buf)

			mi := control.NewMockInvoker(log, &control.MockInvokerConfig{
				UnaryError: tc.uErr,
				UnaryResponse: control.MockMSResponse("host1", nil,
					&ctlpb.SystemQueryResp{Members: tc.members}),
			})
			cmd := storageFormatCmd{}
			cmd.log = log
			cmd.Reformat = tc.reformat
			cmd.ctlInvoker = mi

			ok, err := cmd.shouldReformatSystem(context.Background(), tc.rankList)
			common.CmpErr(t, tc.expErr, err)
			common.AssertEqual(t, tc.expOK, ok, name)
		})
	}
}

//func TestScanDisplay(t *testing.T) {
//	//mockController := storage.MockNvmeController()
//
//	for name, tc := range map[string]struct {
//		scanResp  *StorageScanResp
//		summary   bool
//		expOut    string
//		expErrMsg string
//	}{
//		"typical scan": {
//			scanResp: MockScanResp(MockCtrlrs, MockScmModules, MockScmNamespaces, MockServers),
//			expOut:   fmt.Sprintf("\n 1.2.3.[4-5]\nSCM namespaces:\nBlock Device   Socket ID       Capacity\n------------   ---------       --------"),
//				"pmem1          1               2.90TB  \n"+
//				"NVMe controllers and namespaces:\n"+
//				"PCI Address    Model   FW Revision     Socket ID       Capacity\n"+
//				"-----------    -----   -----------     ---------       --------\n"+
//				"%s      %s %s         %d               %d.00GB  \n",
//				mockController.PciAddr, mockController.Model, mockController.FwRev,
//				mockController.SocketID, mockController.Namespaces[0].Size),
//		},
//		"summary scan": {
//			scanResp: MockScanResp(MockCtrlrs, MockScmModules, MockScmNamespaces, MockServers),
//			summary:  true,
//			expOut: fmt.Sprintf("\n HOSTS\t\tSCM\t\t\tNVME\t\n -----\t\t---\t\t\t----\t\n 1.2"+
//				".3.[4-5]\t2.90TB (1 namespace)\t%d.00GB (1 controller)",
//				mockController.Namespaces[0].Size),
//		},
//		"scm scan with pmem namespaces": {
//			scanResp: MockScanResp(nil, MockScmModules, MockScmNamespaces, MockServers),
//			expOut: "\n 1.2.3.[4-5]\n\tSCM Namespaces:\n\t\tDevice:pmem1 Socket:1 " +
//				"Capacity:2.90TB\n\tNVMe controllers and namespaces:\n\t\tnone\n",
//		},
//		"summary scm scan with pmem namespaces": {
//			scanResp: MockScanResp(nil, MockScmModules, MockScmNamespaces, MockServers),
//			summary:  true,
//			expOut: "\n HOSTS\t\tSCM\t\t\tNVME\t\n -----\t\t---\t\t\t----\t\n 1.2.3." +
//				"[4-5]\t2.90TB (1 namespace)\t0.00B (0 controllers)",
//		},
//		"scm scan without pmem namespaces": {
//			scanResp: MockScanResp(nil, MockScmModules, nil, MockServers),
//			expOut: "\n 1.2.3.[4-5]\n\tSCM Modules:\n\t\tPhysicalID:12345 " +
//				"Capacity:12.06KB Location:(socket:4 memctrlr:3 chan:1 pos:2)\n" +
//				"\tNVMe controllers and namespaces:\n\t\tnone\n",
//		},
//		"summary scm scan without pmem namespaces": {
//			scanResp: MockScanResp(nil, MockScmModules, nil, MockServers),
//			summary:  true,
//			expOut: "\n HOSTS\t\tSCM\t\t\tNVME\t\n -----\t\t---\t\t\t----\t\n 1.2.3." +
//				"[4-5]\t12.06KB (1 module)\t0.00B (0 controllers)",
//		},
//		"nvme scan": {
//			scanResp: MockScanResp(MockCtrlrs, nil, nil, MockServers),
//			expOut: fmt.Sprintf("\n 1.2.3.[4-5]\n\tSCM Modules:\n\t\tnone\n\t"+
//				"NVMe controllers and namespaces:\n\t\t"+
//				"PCI:%s Model:%s FW:%s Socket:%d Capacity:%d.00GB\n",
//				mockController.PciAddr, mockController.Model, mockController.FwRev,
//				mockController.SocketID, mockController.Namespaces[0].Size),
//		},
//		"summary nvme scan": {
//			scanResp: MockScanResp(MockCtrlrs, nil, nil, MockServers),
//			summary:  true,
//			expOut: fmt.Sprintf("\n HOSTS\t\tSCM\t\t\tNVME\t\n -----\t\t---\t\t\t----\t\n 1.2"+
//				".3.[4-5]\t0.00B (0 modules)\t%d.00GB (1 controller)",
//				mockController.Namespaces[0].Size),
//		},
//	} {
//		t.Run(name, func(t *testing.T) {
//			out, err := scanCmdDisplay(tc.scanResp)
//			ExpectError(t, err, tc.expErrMsg, name)
//			if diff := cmp.Diff(tc.expOut, out); diff != "" {
//				t.Fatalf("unexpected output (-want, +got):\n%s\n", diff)
//			}
//		})
//	}
//}
