/*
Copyright 2019 The Vitess Authors.

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

// Package vtctlclienttest contains the testsuite against which each
// RPC implementation of the vtctlclient interface must be tested.
package vtctlclienttest

// NOTE: This file is not test-only code because it is referenced by tests in
//			 other packages and therefore it has to be regularly visible.

// NOTE: This code is in its own package such that its dependencies (e.g.
//       zookeeper) won't be drawn into production binaries as well.

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"vitess.io/vitess/go/vt/logutil"
	"vitess.io/vitess/go/vt/topo"
	"vitess.io/vitess/go/vt/topo/memorytopo"
	"vitess.io/vitess/go/vt/vtctl/vtctlclient"
	"vitess.io/vitess/go/vt/vttablet/tmclienttest"

	// import the gRPC client implementation for tablet manager
	_ "vitess.io/vitess/go/vt/vttablet/grpctmclient"

	topodatapb "vitess.io/vitess/go/vt/proto/topodata"
)

func init() {
	// enforce we will use the right protocol (gRPC) (note the
	// client is unused, but it is initialized, so it needs to exist)
	tmclienttest.SetProtocol("go.vt.vtctl.vtctlclienttest", "grpc")
}

// CreateTopoServer returns the test topo server properly configured
func CreateTopoServer(t *testing.T, ctx context.Context) *topo.Server {
	return memorytopo.NewServer(ctx, "cell1")
}

// TestSuite runs the test suite on the given topo server and client
func TestSuite(t *testing.T, ts *topo.Server, client vtctlclient.VtctlClient) {
	ctx := context.Background()

	// Create a fake tablet
	tablet := &topodatapb.Tablet{
		Alias:         &topodatapb.TabletAlias{Cell: "cell1", Uid: 1},
		Hostname:      "localhost",
		MysqlHostname: "localhost",
		PortMap: map[string]int32{
			"vt": 3333,
		},
		PrimaryTermStartTime: logutil.TimeToProto(time.Date(1970, 1, 1, 1, 1, 1, 1, time.UTC)),
		Tags:                 map[string]string{"tag": "value"},
		Keyspace:             "test_keyspace",
		Type:                 topodatapb.TabletType_PRIMARY,
	}
	tablet.MysqlPort = 3334
	if err := ts.CreateTablet(ctx, tablet); err != nil {
		t.Errorf("CreateTablet: %v", err)
	}

	// run a command that's gonna return something on the log channel
	stream, err := client.ExecuteVtctlCommand(ctx, []string{"ListAllTablets", "cell1"}, 30*time.Second)
	if err != nil {
		t.Fatalf("Remote error: %v", err)
	}

	got, err := stream.Recv()
	if err != nil {
		t.Fatalf("failed to get first line: %v", err)
	}
	expected := "cell1-0000000001 test_keyspace <null> primary localhost:3333 localhost:3334 [tag: \"value\"] 1970-01-01T01:01:01Z\n"
	if logutil.EventString(got) != expected {
		t.Errorf("Got unexpected log line '%v' expected '%v'", got.String(), expected)
	}

	got, err = stream.Recv()
	if err != io.EOF {
		t.Errorf("Didn't get end of log stream: %v %v", got, err)
	}

	// run a command that's gonna fail
	stream, err = client.ExecuteVtctlCommand(ctx, []string{"ListAllTablets", "cell2"}, 30*time.Second)
	if err != nil {
		t.Fatalf("Remote error: %v", err)
	}

	_, err = stream.Recv()
	expected = "node doesn't exist"
	if err == nil || !strings.Contains(err.Error(), expected) {
		t.Fatalf("Unexpected remote error, got: '%v' was expecting to find '%v'", err, expected)
	}

	// run a command that's gonna panic
	stream, err = client.ExecuteVtctlCommand(ctx, []string{"Panic"}, 30*time.Second)
	if err != nil {
		t.Fatalf("Remote error: %v", err)
	}

	_, err = stream.Recv()
	expected1 := "this command panics on purpose"
	expected2 := "uncaught vtctl panic"
	if err == nil || !strings.Contains(err.Error(), expected1) || !strings.Contains(err.Error(), expected2) {
		t.Fatalf("Unexpected remote error, got: '%v' was expecting to find '%v' and '%v'", err, expected1, expected2)
	}

	// and clean up the tablet
	if err := ts.DeleteTablet(ctx, tablet.Alias); err != nil {
		t.Errorf("DeleteTablet: %v", err)
	}
}
