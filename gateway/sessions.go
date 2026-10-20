package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Gateway implements the CAMARA QoD sessions resource on top of the PCF.
type Gateway struct {
	PCF      *PCF
	Profiles map[string]Profile
	Network  Network
	Callback string // base URL the PCF calls back on (our /pcf-events)
	Sink     *http.Client

	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	info         SessionInfo
	appSessionID string
	token        string
	timer        *time.Timer
}

func (g *Gateway) createSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSession
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, invalid(err.Error()))
		return
	}
	profile, ok := g.Profiles[req.QosProfile]
	if !ok {
		writeError(w, &ErrorInfo{http.StatusUnprocessableEntity, "QUALITY_ON_DEMAND.QOS_PROFILE_NOT_APPLICABLE",
			"The requested QoS profile is not offered: " + req.QosProfile})
		return
	}
	if req.Duration > profile.MaxDuration {
		writeError(w, &ErrorInfo{http.StatusBadRequest, "QUALITY_ON_DEMAND.DURATION_OUT_OF_RANGE",
			"The requested duration is out of the allowed range for the QoS profile"})
		return
	}
	id := uuid.NewString()
	ctx, err := appSessionFor(req, profile, g.Network, g.Callback+"/"+id)
	if err != nil {
		writeError(w, err)
		return
	}
	if g.conflict(ctx.AscReqData.UeIpv4, req.ApplicationServer) {
		writeError(w, &ErrorInfo{http.StatusConflict, "CONFLICT", "There is another existing session for the same device"})
		return
	}
	log.Printf("POST /sessions  device %s  %s  %ds", ctx.AscReqData.UeIpv4, req.QosProfile, req.Duration)
	log.Printf("  -> PCF POST %s", appSessions)
	log.Printf("     medType %s  max DL %s  min DL %s", profile.MedType, profile.MarBwDl, profile.MirBwDl)
	log.Printf("     %s", ctx.AscReqData.MedComponents["1"].MedSubComps["1"].FDescs[0])

	appID, err := g.PCF.Create(ctx)
	if err != nil {
		log.Printf("  <- PCF failed: %v", err)
		writeError(w, &ErrorInfo{http.StatusNotFound, "IDENTIFIER_NOT_FOUND",
			"The network has no PDU session for this device"})
		return
	}
	log.Printf("  <- PCF 201 Created  app session %s", appID)

	// The PCF accepted the policy and pushed it to the SMF and UPF, so the QoS is in place.
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(time.Duration(req.Duration) * time.Second)
	s := &session{appSessionID: appID, info: SessionInfo{
		SessionID: id, Device: req.Device, ApplicationServer: req.ApplicationServer,
		DevicePorts: req.DevicePorts, ApplicationServerPorts: req.ApplicationServerPorts,
		QosProfile: req.QosProfile, Sink: req.Sink, Duration: req.Duration,
		StartedAt: &now, ExpiresAt: &expires, QosStatus: Available,
	}}
	if req.SinkCredential != nil {
		s.token = req.SinkCredential.AccessToken
	}
	g.mu.Lock()
	if g.sessions == nil {
		g.sessions = map[string]*session{}
	}
	g.sessions[id] = s
	s.timer = time.AfterFunc(expires.Sub(time.Now()), func() { g.end(id, DurationExpired) })
	info := s.info
	g.mu.Unlock()

	log.Printf("session %s  %s until %s", id[:8], Available, expires.Local().Format(time.TimeOnly))
	writeJSON(w, http.StatusCreated, info)
	g.notify(s, info)
}

func (g *Gateway) getSession(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	s, ok := g.sessions[r.PathValue("sessionId")]
	var info SessionInfo
	if ok {
		info = s.info
	}
	g.mu.Unlock()
	if !ok {
		writeError(w, notFound())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (g *Gateway) deleteSession(w http.ResponseWriter, r *http.Request) {
	if !g.end(r.PathValue("sessionId"), DeleteRequested) {
		writeError(w, notFound())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (g *Gateway) extendSession(w http.ResponseWriter, r *http.Request) {
	var req ExtendSessionDuration
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, invalid(err.Error()))
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.sessions[r.PathValue("sessionId")]
	if !ok {
		writeError(w, notFound())
		return
	}
	if s.info.QosStatus != Available {
		writeError(w, &ErrorInfo{http.StatusConflict, "QUALITY_ON_DEMAND.SESSION_EXTENSION_NOT_ALLOWED",
			"Extending the session duration is not allowed in the current state. The session must be AVAILABLE."})
		return
	}
	// The overall duration is capped at the profile's maximum.
	s.info.Duration = min(s.info.Duration+req.RequestedAdditionalDuration, g.Profiles[s.info.QosProfile].MaxDuration)
	expires := s.info.StartedAt.Add(time.Duration(s.info.Duration) * time.Second)
	s.info.ExpiresAt = &expires
	s.timer.Reset(time.Until(expires))
	log.Printf("session %s  extended to %ds, until %s", s.info.SessionID[:8], s.info.Duration, expires.Local().Format(time.TimeOnly))
	writeJSON(w, http.StatusOK, s.info)
}

func (g *Gateway) retrieveSessions(w http.ResponseWriter, r *http.Request) {
	var req RetrieveSessionsInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, invalid(err.Error()))
		return
	}
	ue, err := deviceIPv4(req.Device)
	if err != nil {
		writeError(w, err)
		return
	}
	found := []SessionInfo{}
	g.mu.Lock()
	for _, s := range g.sessions {
		if ip, _ := deviceIPv4(s.info.Device); ip == ue {
			found = append(found, s.info)
		}
	}
	g.mu.Unlock()
	writeJSON(w, http.StatusOK, found)
}

// pcfTermination receives the PCF's termination notification (TS 29.514
// Npcf_PolicyAuthorization_Notify), e.g. when the UE's PDU session is released.
func (g *Gateway) pcfTermination(w http.ResponseWriter, r *http.Request) {
	log.Printf("<- PCF terminated the app session of session %.8s", r.PathValue("sessionId"))
	g.end(r.PathValue("sessionId"), NetworkTerminated)
	w.WriteHeader(http.StatusNoContent)
}

// end releases a session for the given reason: it deletes the PCF app session
// (unless the network already did) and tells the sink.
func (g *Gateway) end(id, reason string) bool {
	g.mu.Lock()
	s, ok := g.sessions[id]
	if !ok {
		g.mu.Unlock()
		return false
	}
	wasAvailable := s.info.QosStatus == Available
	s.timer.Stop()
	if reason == NetworkTerminated {
		// Keep the session readable by clients that poll instead of using a sink;
		// the spec says at least 360 s.
		now := time.Now().UTC().Truncate(time.Second)
		s.info.QosStatus, s.info.StatusInfo, s.info.ExpiresAt = Unavailable, reason, &now
		s.timer = time.AfterFunc(360*time.Second, func() { g.forget(id) })
	} else {
		delete(g.sessions, id)
	}
	g.mu.Unlock()
	if !wasAvailable {
		return true // already UNAVAILABLE: nothing left to release or report
	}
	log.Printf("session %s  %s  %s", id[:8], Unavailable, reason)
	if reason != NetworkTerminated {
		if err := g.PCF.Delete(s.appSessionID); err != nil {
			log.Printf("  <- PCF delete failed: %v", err)
		} else {
			log.Printf("  -> PCF deleted app session %s", s.appSessionID)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	info := s.info
	info.QosStatus, info.StatusInfo, info.ExpiresAt = Unavailable, reason, &now
	g.notify(s, info)
	return true
}

func (g *Gateway) forget(id string) {
	g.mu.Lock()
	delete(g.sessions, id)
	g.mu.Unlock()
}

func (g *Gateway) conflict(ue string, server ApplicationServer) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, s := range g.sessions {
		if ip, _ := deviceIPv4(s.info.Device); ip == ue && s.info.ApplicationServer == server && s.info.QosStatus == Available {
			return true
		}
	}
	return false
}

// notify posts a QOS_STATUS_CHANGED CloudEvent to the session's sink, if any.
func (g *Gateway) notify(s *session, info SessionInfo) {
	if info.Sink == "" {
		return
	}
	event := CloudEvent{
		ID: uuid.NewString(), Source: "/quality-on-demand/v1/sessions/" + info.SessionID,
		Type: "org.camaraproject.quality-on-demand.v1.qos-status-changed", SpecVersion: "1.0",
		DataContentType: "application/json", Time: time.Now().UTC(),
		Data: EventData{SessionID: info.SessionID, QosStatus: info.QosStatus, StatusInfo: info.StatusInfo},
	}
	body, _ := json.Marshal(event)
	go func() {
		req, _ := http.NewRequest(http.MethodPost, info.Sink, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/cloudevents+json")
		if s.token != "" {
			req.Header.Set("Authorization", "Bearer "+s.token)
		}
		resp, err := g.Sink.Do(req)
		if err != nil {
			log.Printf("  -> sink failed: %v", err)
			return
		}
		resp.Body.Close()
		log.Printf("  -> sink QOS_STATUS_CHANGED %s: %s", strings.TrimSpace(info.QosStatus+" "+info.StatusInfo), resp.Status)
	}()
}

func sinkClient(insecure bool) *http.Client {
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, // lab sinks use self-signed certificates
	}}
}

func notFound() *ErrorInfo {
	return &ErrorInfo{http.StatusNotFound, "NOT_FOUND", "The specified resource is not found."}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var e *ErrorInfo
	if !errors.As(err, &e) {
		e = &ErrorInfo{http.StatusInternalServerError, "INTERNAL", err.Error()}
	}
	writeJSON(w, e.Status, e)
}
