# CAMARA Quality-on-Demand on a public 5G core

**Objective:** show, end to end, that one standard API call really changes how a 5G network
treats traffic, using only open-source software. The demo follows a single CAMARA request from
the API, through 3GPP policy (gateway as our NEF → PCF → SMF), to the packet and QoS rules the UPF
installs, and then removes them again when the session is deleted.

A small, end-to-end lab. An application asks for better QoS through the
[CAMARA Quality-On-Demand API](https://github.com/camaraproject/QualityOnDemand) (v1.1.0), and an
unmodified [free5GC](https://github.com/free5gc/free5gc) v4.2.3 core enforces it for a
[UERANSIM](https://github.com/aligungr/UERANSIM) UE.

```
app ──CAMARA QoD──▶ gateway ──N5──▶ PCF ──N7──▶ SMF ──N4/PFCP──▶ UPF ◀──N3── gNB ◀── UE
                    our NEF          Npcf_PolicyAuthorization      new PDR + QER (gtp5g)
                    (this repo)
```

## The gateway is our NEF

In an operator network, an outside application never talks to the core directly. It goes through
the **NEF** (Network Exposure Function), which receives the QoS request and turns it into a
policy request to the PCF. The free5GC NEF does not implement that QoS API, so in this lab the
**gateway plays the role of the NEF**: it is the single entry point from the outside world, and it
calls the PCF's Npcf_PolicyAuthorization service, the same service a NEF would call.

```
operator network   app ──CAMARA──▶ API gateway ──TS 29.122──▶ NEF ──N30──▶ PCF
this lab           app ──CAMARA──────────────▶ gateway (our NEF) ──N5───▶ PCF
```

What it does like a NEF, and what it leaves out:

| A real NEF                                                    | Our gateway                                              |
|---------------------------------------------------------------|----------------------------------------------------------|
| single, controlled entry point for outside apps               | yes: apps only see the CAMARA API                        |
| creates the PCF app session (Npcf_PolicyAuthorization)        | yes: same service, same `AppSessionContext`              |
| relays status changes back to the app                         | yes: PCF events become CAMARA CloudEvents                |
| exposes the 3GPP API (TS 29.122 AsSessionWithQoS)             | no: it exposes CAMARA directly, with no API in between   |
| authorizes each app (OAuth2, SLAs)                            | no: any caller is accepted                               |
| maps external identifiers (phone number, GPSI) to the UE      | no: the device is given by its IP address                |
| finds the right PCF through the BSF                           | no: one PCF, set with `-pcf`                             |

So, strictly, it calls the PCF as a trusted AF over N5, where a NEF would use N30. The service and
the message are the same, so the PCF, SMF and UPF behave exactly as they would behind a real NEF.

## The translation

The gateway in [gateway/](gateway/) translates each CAMARA call into the 3GPP policy API:

| CAMARA (`POST /sessions`)        | PCF `AppSessionContext` (TS 29.514)                                  |
|----------------------------------|----------------------------------------------------------------------|
| `device.ipv4Address`             | `ueIpv4`: the PCF uses it to find the UE's PDU session                |
| `applicationServer`, ports       | `fDescs`: `permit out ip from <server> <ports> to <device> <ports>`   |
| `qosProfile`                     | media component from [profiles.yaml](gateway/profiles.yaml): `medType` (sets the 5QI) and bit rates |
| `duration`                       | a timer in the gateway that deletes the app session                  |
| `sink`, `sinkCredential`         | `qos-status-changed` CloudEvents, with the bearer token              |

It implements create, get, delete, extend and retrieve-sessions. Every request is validated
against the official OpenAPI document in [spec/](spec/). When the PCF reports that the PDU
session ended, the gateway sends `UNAVAILABLE / NETWORK_TERMINATED`.

## Limits of this lab

- **The gateway stands in for the NEF.** The free5GC NEF does not implement TS 29.122
  AsSessionWithQoS, so the gateway takes its place; see [The gateway is our NEF](#the-gateway-is-our-nef)
  for what it leaves out.
- **The UE is not told.** Upstream SMF installs the new rules in the UPF (PFCP) but sends
  no PDU Session Modification to the UE. Downlink enforcement happens in the UPF.
- **SBI OAuth2 is off** (`oauth: false` in `nrfcfg.yaml`), so the gateway needs no NRF token.
- **One UE, IPv4 only.** The device is identified by `device.ipv4Address`. With several UEs,
  upstream PCF 4.2.3 can bind an app session to the wrong UE.
- **Sessions live in memory.** Restarting the gateway forgets them.
- Bit rates show up in the UPF as Kbps, with 1 Mbps = 1024 Kbps.

## Prerequisites

Tested on Ubuntu 24.04, kernel 6.8, Go 1.26.2, Node.js 20, MongoDB 8.2, with passwordless `sudo`.

```bash
sudo apt-get install -y build-essential linux-headers-$(uname -r) git curl jq openssl python3
```

Three more things that are not in Ubuntu's archive at the right version:

- **Go 1.26.2 or newer.** free5GC v4.2.3 requires it in every `go.mod`. Install the tarball from
  [go.dev/dl](https://go.dev/dl/) into `/usr/local/go`, and put `/usr/local/go/bin` and `~/go/bin` on your `PATH`.
- **Node.js 20 and yarn**, to build the webconsole. For example, Node.js from
  [NodeSource](https://github.com/nodesource/distributions), then `sudo npm install -g yarn`.
- **MongoDB**, running on `127.0.0.1:27017`. Install `mongodb-org` (which includes `mongosh`) from
  [MongoDB's apt repository](https://www.mongodb.com/docs/manual/tutorial/install-mongodb-on-ubuntu/),
  then `sudo systemctl enable --now mongod`.

`install-free5gc.sh` installs the apt packages too, and stops with a list if Go, Node.js, yarn or
MongoDB is missing.

## Run it

```bash
./scripts/install-free5gc.sh     # gtp5g, free5GC v4.2.3 (+ webconsole), OAuth off, gogtp5g-tunnel
./scripts/install-ueransim.sh    # UERANSIM v3.3.0
```

Use one terminal per step:

```bash
# 1. core
cd ~/free5gc && sudo ./run.sh
# 2. webconsole, then the demo subscriber (imsi-208930000000001)
~/free5gc/webconsole/bin/webconsole -c ~/free5gc/config/webuicfg.yaml &
./scripts/add-subscriber.sh
# 3. radio
~/UERANSIM/build/nr-gnb -c config/ueransim/gnb.yaml
sudo ~/UERANSIM/build/nr-ue -c config/ueransim/ue.yaml      # brings up uesimtun0
# 4. gateway and a notification sink
cd gateway && go run . -insecure-sink
python3 scripts/sink.py
```

Then the QoD lifecycle:

```bash
./scripts/qod.sh create requests/create-session.json   # 201, qosStatus AVAILABLE
./scripts/corelog.sh                                   # PCF → SMF → UPF in the free5GC log
./scripts/rules.sh                                     # new PDRs + QER in gtp5g
./scripts/qod.sh get
./scripts/qod.sh extend 300
./scripts/qod.sh delete                                # 204, rules removed
```

[requests/create-session.json](requests/create-session.json) uses `10.60.0.1`, the first address
of a freshly started SMF. If your UE got another address (`ip -br addr show uesimtun0`), change it there.

To test the gateway without a core, run `cd gateway && go test ./...`.