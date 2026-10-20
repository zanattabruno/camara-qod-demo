package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePCF records app sessions like the free5gc PCF: 201 + Location, then 204 on delete.
type fakePCF struct {
	mu      sync.Mutex
	created []AppSessionContext
	deleted []string
}

func (f *fakePCF) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == appSessions:
		var ctx AppSessionContext
		json.NewDecoder(r.Body).Decode(&ctx)
		f.created = append(f.created, ctx)
		w.Header().Set("Location", "http://pcf"+appSessions+"/imsi-208930000000001-1")
		w.WriteHeader(http.StatusCreated)
	case strings.HasSuffix(r.URL.Path, "/delete"):
		f.deleted = append(f.deleted, strings.Split(r.URL.Path, "/")[4])
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestGateway(t *testing.T) (*httptest.Server, *fakePCF, chan CloudEvent) {
	t.Helper()
	pcf := &fakePCF{}
	pcfSrv := httptest.NewServer(pcf)
	events := make(chan CloudEvent, 4)
	sink := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e CloudEvent
		json.NewDecoder(r.Body).Decode(&e)
		events <- e
		w.WriteHeader(http.StatusNoContent)
	}))
	profiles, err := loadProfiles("profiles.yaml")
	if err != nil {
		t.Fatal(err)
	}
	validate, err := specValidator("../spec/quality-on-demand.yaml")
	if err != nil {
		t.Fatal(err)
	}
	gw := &Gateway{PCF: &PCF{Root: pcfSrv.URL}, Profiles: profiles,
		Network: Network{"internet", 1, "010203"}, Callback: "http://gw/pcf-events", Sink: sinkClient(true)}
	srv := httptest.NewServer(gw.routes(validate))
	t.Cleanup(func() { srv.Close(); sink.Close(); pcfSrv.Close() })
	sinkURL = sink.URL
	return srv, pcf, events
}

var sinkURL string

func call(t *testing.T, method, url, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, url, err, b)
		}
	}
	return resp.StatusCode
}

func waitEvent(t *testing.T, events chan CloudEvent) CloudEvent {
	t.Helper()
	select {
	case e := <-events:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no event at the sink")
		return CloudEvent{}
	}
}

const createBody = `{
  "device": {"ipv4Address": {"publicAddress": "10.60.0.1", "privateAddress": "10.60.0.1"}},
  "applicationServer": {"ipv4Address": "198.51.100.10"},
  "qosProfile": "QOS_L", "duration": 600, "sink": "%s",
  "sinkCredential": {"credentialType": "ACCESSTOKEN", "accessToken": "t", "accessTokenExpiresUtc": "2030-01-01T00:00:00Z", "accessTokenType": "bearer"}}`

func TestSessionLifecycle(t *testing.T) {
	srv, pcf, events := newTestGateway(t)
	base := srv.URL + "/quality-on-demand/v1/sessions"

	var created SessionInfo
	if code := call(t, "POST", base, strings.Replace(createBody, "%s", sinkURL, 1), &created); code != 201 {
		t.Fatalf("create: %d", code)
	}
	if created.QosStatus != Available || created.StartedAt == nil || created.Duration != 600 {
		t.Fatalf("created: %+v", created)
	}
	if e := waitEvent(t, events); e.Data.QosStatus != Available || e.Type != "org.camaraproject.quality-on-demand.v1.qos-status-changed" {
		t.Fatalf("event: %+v", e)
	}
	if len(pcf.created) != 1 || pcf.created[0].AscReqData.UeIpv4 != "10.60.0.1" {
		t.Fatalf("PCF got %+v", pcf.created)
	}

	if code := call(t, "POST", base, strings.Replace(createBody, "%s", sinkURL, 1), nil); code != 409 {
		t.Fatalf("second session for the same flow: %d, want 409", code)
	}

	var extended SessionInfo
	if code := call(t, "POST", base+"/"+created.SessionID+"/extend", `{"requestedAdditionalDuration": 60}`, &extended); code != 200 || extended.Duration != 660 {
		t.Fatalf("extend: %d %+v", code, extended)
	}

	var got SessionInfo
	if code := call(t, "GET", base+"/"+created.SessionID, "", &got); code != 200 || got.QosStatus != Available {
		t.Fatalf("get: %d %+v", code, got)
	}

	if code := call(t, "DELETE", base+"/"+created.SessionID, "", nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if e := waitEvent(t, events); e.Data.QosStatus != Unavailable || e.Data.StatusInfo != DeleteRequested {
		t.Fatalf("event: %+v", e)
	}
	if len(pcf.deleted) != 1 || pcf.deleted[0] != "imsi-208930000000001-1" {
		t.Fatalf("PCF deletes: %v", pcf.deleted)
	}
	if code := call(t, "GET", base+"/"+created.SessionID, "", nil); code != 404 {
		t.Fatalf("get after delete: %d", code)
	}
}

func TestNetworkTermination(t *testing.T) {
	srv, pcf, events := newTestGateway(t)
	base := srv.URL + "/quality-on-demand/v1/sessions"
	var created SessionInfo
	call(t, "POST", base, strings.Replace(createBody, "%s", sinkURL, 1), &created)
	waitEvent(t, events)

	// The PCF reports that the PDU session is gone.
	if code := call(t, "POST", srv.URL+"/pcf-events/"+created.SessionID, `{"event": "TERMINATION"}`, nil); code != 204 {
		t.Fatalf("termination: %d", code)
	}
	if e := waitEvent(t, events); e.Data.QosStatus != Unavailable || e.Data.StatusInfo != NetworkTerminated {
		t.Fatalf("event: %+v", e)
	}
	// Still readable for polling clients, and deletable without a second event.
	var got SessionInfo
	if code := call(t, "GET", base+"/"+created.SessionID, "", &got); code != 200 || got.StatusInfo != NetworkTerminated {
		t.Fatalf("get: %d %+v", code, got)
	}
	if code := call(t, "DELETE", base+"/"+created.SessionID, "", nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	select {
	case e := <-events:
		t.Fatalf("unexpected event after delete of an UNAVAILABLE session: %+v", e)
	case <-time.After(200 * time.Millisecond):
	}
	if len(pcf.deleted) != 0 {
		t.Fatalf("the PCF already removed the app session, but the gateway deleted it again: %v", pcf.deleted)
	}
}

func TestSpecValidation(t *testing.T) {
	srv, pcf, _ := newTestGateway(t)
	base := srv.URL + "/quality-on-demand/v1/sessions"
	for name, body := range map[string]string{
		"missing qosProfile": `{"device": {"ipv4Address": {"publicAddress": "10.60.0.1", "privateAddress": "10.60.0.1"}}, "applicationServer": {"ipv4Address": "198.51.100.10"}, "duration": 60}`,
		"plain http sink":    `{"device": {"ipv4Address": {"publicAddress": "10.60.0.1", "privateAddress": "10.60.0.1"}}, "applicationServer": {"ipv4Address": "198.51.100.10"}, "qosProfile": "QOS_L", "duration": 60, "sink": "http://x"}`,
		"zero duration":      `{"device": {"ipv4Address": {"publicAddress": "10.60.0.1", "privateAddress": "10.60.0.1"}}, "applicationServer": {"ipv4Address": "198.51.100.10"}, "qosProfile": "QOS_L", "duration": 0}`,
	} {
		var e ErrorInfo
		if code := call(t, "POST", base, body, &e); code != 400 || e.Code != "INVALID_ARGUMENT" {
			t.Errorf("%s: %d %+v", name, code, e)
		}
	}
	var e ErrorInfo
	body := `{"device": {"ipv4Address": {"publicAddress": "10.60.0.1", "privateAddress": "10.60.0.1"}}, "applicationServer": {"ipv4Address": "198.51.100.10"}, "qosProfile": "QOS_XL", "duration": 60}`
	if code := call(t, "POST", base, body, &e); code != 422 {
		t.Errorf("unknown profile: %d %+v", code, e)
	}
	if len(pcf.created) != 0 {
		t.Errorf("invalid requests reached the PCF: %d", len(pcf.created))
	}
}
