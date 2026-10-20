package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile is what a CAMARA QoS profile name means for this network: one
// 3GPP media component (TS 29.514). The PCF picks the 5QI from medType
// (AUDIO -> 1, VIDEO -> 2, DATA -> 9); mar* are maximum and mir* are
// guaranteed bit rates.
type Profile struct {
	MedType     string `yaml:"medType" json:"medType"`
	MarBwDl     string `yaml:"marBwDl" json:"marBwDl,omitempty"`
	MarBwUl     string `yaml:"marBwUl" json:"marBwUl,omitempty"`
	MirBwDl     string `yaml:"mirBwDl" json:"mirBwDl,omitempty"`
	MirBwUl     string `yaml:"mirBwUl" json:"mirBwUl,omitempty"`
	MaxDuration int    `yaml:"maxDuration" json:"-"`
}

func loadProfiles(path string) (map[string]Profile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	profiles := map[string]Profile{}
	return profiles, yaml.Unmarshal(b, &profiles)
}

// Network is the PDU session the device's traffic belongs to.
type Network struct {
	Dnn string
	Sst int
	Sd  string
}

// AppSessionContext is the Npcf_PolicyAuthorization request body (TS 29.514),
// reduced to the fields this gateway sets.
type AppSessionContext struct {
	AscReqData AscReqData `json:"ascReqData"`
}

type AscReqData struct {
	AfAppID       string                    `json:"afAppId"`
	NotifURI      string                    `json:"notifUri"`
	SuppFeat      string                    `json:"suppFeat"`
	UeIpv4        string                    `json:"ueIpv4"`
	Dnn           string                    `json:"dnn"`
	SliceInfo     Snssai                    `json:"sliceInfo"`
	MedComponents map[string]MediaComponent `json:"medComponents"`
}

type Snssai struct {
	Sst int    `json:"sst"`
	Sd  string `json:"sd"`
}

type MediaComponent struct {
	MedCompN int `json:"medCompN"`
	Profile
	FStatus     string                       `json:"fStatus"`
	MedSubComps map[string]MediaSubComponent `json:"medSubComps"`
}

type MediaSubComponent struct {
	FNum    int      `json:"fNum"`
	FDescs  []string `json:"fDescs"`
	FStatus string   `json:"fStatus"`
}

// appSessionFor translates a CAMARA createSession request into the PCF's
// AppSessionContext: which UE (its IP), which flow (an IPFilterRule) and
// how much (the bit rates of the QoS profile).
func appSessionFor(req CreateSession, p Profile, nw Network, notifURI string) (AppSessionContext, error) {
	ue, err := deviceIPv4(req.Device)
	if err != nil {
		return AppSessionContext{}, err
	}
	server := req.ApplicationServer.IPv4Address
	if server == "" {
		return AppSessionContext{}, invalid("applicationServer.ipv4Address is required: this network is IPv4 only")
	}
	// IPFilterRule from the server to the device; the core installs it for both directions.
	flow := fmt.Sprintf("permit out ip from %s%s to %s%s",
		server, ports(req.ApplicationServerPorts), ue, ports(req.DevicePorts))
	return AppSessionContext{AscReqData: AscReqData{
		AfAppID:   "camara-qod",
		NotifURI:  notifURI,
		SuppFeat:  "0",
		UeIpv4:    ue,
		Dnn:       nw.Dnn,
		SliceInfo: Snssai{Sst: nw.Sst, Sd: nw.Sd},
		MedComponents: map[string]MediaComponent{"1": {
			MedCompN: 1,
			Profile:  p,
			FStatus:  "ENABLED",
			MedSubComps: map[string]MediaSubComponent{"1": {
				FNum: 1, FDescs: []string{flow}, FStatus: "ENABLED",
			}},
		}},
	}}, nil
}

// deviceIPv4 returns the address the UE got from the SMF. Without NAT the
// private and public addresses are the same.
func deviceIPv4(d *Device) (string, error) {
	switch {
	case d == nil:
		return "", &ErrorInfo{http.StatusUnprocessableEntity, "MISSING_IDENTIFIER", "The device cannot be identified."}
	case d.IPv4Address != nil && d.IPv4Address.PrivateAddress != "":
		return d.IPv4Address.PrivateAddress, nil
	case d.IPv4Address != nil && d.IPv4Address.PublicAddress != "":
		return d.IPv4Address.PublicAddress, nil
	}
	return "", &ErrorInfo{http.StatusUnprocessableEntity, "UNSUPPORTED_IDENTIFIER",
		"This gateway identifies devices by device.ipv4Address only."}
}

// ports renders a CAMARA PortsSpec in IPFilterRule syntax, e.g. " 5000,6000-6010".
func ports(spec *PortsSpec) string {
	if spec == nil {
		return ""
	}
	var parts []string
	for _, p := range spec.Ports {
		parts = append(parts, fmt.Sprint(p))
	}
	for _, r := range spec.Ranges {
		parts = append(parts, fmt.Sprintf("%d-%d", r.From, r.To))
	}
	return " " + strings.Join(parts, ",")
}

func invalid(msg string) *ErrorInfo {
	return &ErrorInfo{http.StatusBadRequest, "INVALID_ARGUMENT", msg}
}
