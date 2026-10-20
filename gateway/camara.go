package main

import "time"

// The subset of the CAMARA Quality-On-Demand v1.1.0 schemas this gateway uses
// (spec/quality-on-demand.yaml, components/schemas).

type Device struct {
	PhoneNumber             string      `json:"phoneNumber,omitempty"`
	NetworkAccessIdentifier string      `json:"networkAccessIdentifier,omitempty"`
	IPv4Address             *DeviceIPv4 `json:"ipv4Address,omitempty"`
	IPv6Address             string      `json:"ipv6Address,omitempty"`
}

type DeviceIPv4 struct {
	PublicAddress  string `json:"publicAddress,omitempty"`
	PrivateAddress string `json:"privateAddress,omitempty"`
	PublicPort     *int   `json:"publicPort,omitempty"`
}

type ApplicationServer struct {
	IPv4Address string `json:"ipv4Address,omitempty"`
	IPv6Address string `json:"ipv6Address,omitempty"`
}

type PortRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type PortsSpec struct {
	Ranges []PortRange `json:"ranges,omitempty"`
	Ports  []int       `json:"ports,omitempty"`
}

type SinkCredential struct {
	CredentialType string `json:"credentialType"`
	AccessToken    string `json:"accessToken,omitempty"`
}

type CreateSession struct {
	Device                 *Device           `json:"device,omitempty"`
	ApplicationServer      ApplicationServer `json:"applicationServer"`
	DevicePorts            *PortsSpec        `json:"devicePorts,omitempty"`
	ApplicationServerPorts *PortsSpec        `json:"applicationServerPorts,omitempty"`
	QosProfile             string            `json:"qosProfile"`
	Sink                   string            `json:"sink,omitempty"`
	SinkCredential         *SinkCredential   `json:"sinkCredential,omitempty"`
	Duration               int               `json:"duration"`
}

type ExtendSessionDuration struct {
	RequestedAdditionalDuration int `json:"requestedAdditionalDuration"`
}

type RetrieveSessionsInput struct {
	Device *Device `json:"device,omitempty"`
}

// QosStatus values.
const (
	Requested   = "REQUESTED"
	Available   = "AVAILABLE"
	Unavailable = "UNAVAILABLE"
)

// StatusInfo values, the reason a session became UNAVAILABLE.
const (
	DurationExpired   = "DURATION_EXPIRED"
	NetworkTerminated = "NETWORK_TERMINATED"
	DeleteRequested   = "DELETE_REQUESTED"
)

type SessionInfo struct {
	SessionID              string            `json:"sessionId"`
	Device                 *Device           `json:"device,omitempty"`
	ApplicationServer      ApplicationServer `json:"applicationServer"`
	DevicePorts            *PortsSpec        `json:"devicePorts,omitempty"`
	ApplicationServerPorts *PortsSpec        `json:"applicationServerPorts,omitempty"`
	QosProfile             string            `json:"qosProfile"`
	Sink                   string            `json:"sink,omitempty"`
	Duration               int               `json:"duration"`
	StartedAt              *time.Time        `json:"startedAt,omitempty"`
	ExpiresAt              *time.Time        `json:"expiresAt,omitempty"`
	QosStatus              string            `json:"qosStatus"`
	StatusInfo             string            `json:"statusInfo,omitempty"`
}

type ErrorInfo struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorInfo) Error() string { return e.Code + ": " + e.Message }

// CloudEvent is the notification sent to the sink (EventQosStatusChanged).
type CloudEvent struct {
	ID              string    `json:"id"`
	Source          string    `json:"source"`
	Type            string    `json:"type"`
	SpecVersion     string    `json:"specversion"`
	DataContentType string    `json:"datacontenttype"`
	Time            time.Time `json:"time"`
	Data            EventData `json:"data"`
}

type EventData struct {
	SessionID  string `json:"sessionId"`
	QosStatus  string `json:"qosStatus"`
	StatusInfo string `json:"statusInfo,omitempty"`
}
