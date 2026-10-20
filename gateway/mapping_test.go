package main

import (
	"errors"
	"testing"
)

func TestAppSessionFor(t *testing.T) {
	req := CreateSession{
		Device:                 &Device{IPv4Address: &DeviceIPv4{PublicAddress: "10.60.0.1", PrivateAddress: "10.60.0.1"}},
		ApplicationServer:      ApplicationServer{IPv4Address: "198.51.100.10"},
		DevicePorts:            &PortsSpec{Ports: []int{6000}},
		ApplicationServerPorts: &PortsSpec{Ports: []int{5000}, Ranges: []PortRange{{5010, 5020}}},
		QosProfile:             "QOS_L",
		Duration:               600,
	}
	p := Profile{MedType: "VIDEO", MarBwDl: "20 Mbps", MirBwDl: "10 Mbps"}
	ctx, err := appSessionFor(req, p, Network{"internet", 1, "010203"}, "http://gw/pcf-events/x")
	if err != nil {
		t.Fatal(err)
	}
	asc := ctx.AscReqData
	if asc.UeIpv4 != "10.60.0.1" || asc.Dnn != "internet" || asc.SliceInfo != (Snssai{1, "010203"}) {
		t.Fatalf("wrong PDU session binding: %+v", asc)
	}
	comp := asc.MedComponents["1"]
	if comp.MedType != "VIDEO" || comp.MarBwDl != "20 Mbps" || comp.MirBwDl != "10 Mbps" {
		t.Fatalf("wrong media component: %+v", comp)
	}
	want := "permit out ip from 198.51.100.10 5000,5010-5020 to 10.60.0.1 6000"
	if got := comp.MedSubComps["1"].FDescs; len(got) != 1 || got[0] != want {
		t.Fatalf("fDescs = %q, want %q", got, want)
	}
}

func TestAppSessionForWithoutPorts(t *testing.T) {
	req := CreateSession{
		Device:            &Device{IPv4Address: &DeviceIPv4{PublicAddress: "10.60.0.1", PublicPort: new(int)}},
		ApplicationServer: ApplicationServer{IPv4Address: "198.51.100.0/24"},
	}
	ctx, err := appSessionFor(req, Profile{}, Network{}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "permit out ip from 198.51.100.0/24 to 10.60.0.1"
	if got := ctx.AscReqData.MedComponents["1"].MedSubComps["1"].FDescs[0]; got != want {
		t.Fatalf("fDesc = %q, want %q", got, want)
	}
}

func TestDeviceIdentifiers(t *testing.T) {
	for name, tc := range map[string]struct {
		device *Device
		code   string
	}{
		"no device":    {nil, "MISSING_IDENTIFIER"},
		"phone number": {&Device{PhoneNumber: "+123456789"}, "UNSUPPORTED_IDENTIFIER"},
	} {
		_, err := deviceIPv4(tc.device)
		var e *ErrorInfo
		if !errors.As(err, &e) || e.Code != tc.code || e.Status != 422 {
			t.Errorf("%s: got %v, want 422 %s", name, err, tc.code)
		}
	}
}
