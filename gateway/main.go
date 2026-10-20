// Command gateway exposes the CAMARA Quality-On-Demand API v1.1.0 and fulfils
// it with the 5G core's PCF over N5 (3GPP TS 29.514 Npcf_PolicyAuthorization).
//
//	app --CAMARA QoD--> gateway --N5--> PCF --N7--> SMF --N4--> UPF
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:9091", "address of the CAMARA API")
	pcf := flag.String("pcf", "http://127.0.0.7:8000", "PCF SBI root (free5gc default)")
	spec := flag.String("spec", "../spec/quality-on-demand.yaml", "CAMARA OpenAPI document used to validate requests")
	profiles := flag.String("profiles", "profiles.yaml", "QoS profile catalogue")
	dnn := flag.String("dnn", "internet", "DNN of the device's PDU session")
	sst := flag.Int("sst", 1, "slice/service type of the device's PDU session")
	sd := flag.String("sd", "010203", "slice differentiator of the device's PDU session")
	insecureSink := flag.Bool("insecure-sink", false, "accept self-signed sink certificates (lab only)")
	flag.Parse()
	log.SetFlags(log.Ltime)

	catalogue, err := loadProfiles(*profiles)
	if err != nil {
		log.Fatal(err)
	}
	validate, err := specValidator(*spec)
	if err != nil {
		log.Fatal(err)
	}
	gw := &Gateway{
		PCF:      &PCF{Root: *pcf},
		Profiles: catalogue,
		Network:  Network{Dnn: *dnn, Sst: *sst, Sd: *sd},
		Callback: "http://" + *listen + "/pcf-events",
		Sink:     sinkClient(*insecureSink),
	}

	// 5G core functions talk cleartext HTTP/2 (h2c), as the SBI requires; API clients use HTTP/1.1.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Addr: *listen, Handler: gw.routes(validate), Protocols: protocols}

	log.Printf("CAMARA QoD v1.1.0  http://%s/quality-on-demand/v1", *listen)
	log.Printf("PCF               %s", *pcf)
	log.Fatal(server.ListenAndServe())
}

func (g *Gateway) routes(validate func(http.Handler) http.Handler) http.Handler {
	camara := http.NewServeMux()
	camara.HandleFunc("POST /quality-on-demand/v1/sessions", g.createSession)
	camara.HandleFunc("GET /quality-on-demand/v1/sessions/{sessionId}", g.getSession)
	camara.HandleFunc("DELETE /quality-on-demand/v1/sessions/{sessionId}", g.deleteSession)
	camara.HandleFunc("POST /quality-on-demand/v1/sessions/{sessionId}/extend", g.extendSession)
	camara.HandleFunc("POST /quality-on-demand/v1/retrieve-sessions", g.retrieveSessions)

	mux := http.NewServeMux()
	mux.Handle("/quality-on-demand/v1/", validate(camara))
	mux.HandleFunc("POST /pcf-events/{sessionId}", g.pcfTermination)
	return mux
}
