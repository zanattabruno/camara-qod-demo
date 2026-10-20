# CAMARA Quality-on-Demand on a public 5G core

**Objective:** show, end to end, that one standard API call really changes how a 5G network
treats traffic, using only open-source software. The demo follows a single CAMARA request from
the API, through 3GPP policy (gateway as AF → PCF → SMF), to the packet and QoS rules the UPF
installs, and then removes them again when the session is deleted.

A small, end-to-end lab. An application asks for better QoS through the
[CAMARA Quality-On-Demand API](https://github.com/camaraproject/QualityOnDemand) (v1.1.0), and an
unmodified [free5GC](https://github.com/free5gc/free5gc) v4.2.3 core enforces it for a
[UERANSIM](https://github.com/aligungr/UERANSIM) UE.

```
app ──CAMARA QoD──▶ gateway (AF) ──N5──▶ PCF ──N7──▶ SMF ──N4/PFCP──▶ UPF ◀──N3── gNB ◀── UE
                    this repo            Npcf_PolicyAuthorization      new PDR + QER (gtp5g)
```

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

- **No NEF in the path.** Operators expose QoS through the NEF (TS 29.122 AsSessionWithQoS),
  but the free5GC NEF does not implement it. The gateway acts as a trusted AF and calls the PCF directly.
- **The UE is not told.** Upstream SMF installs the new rules in the UPF (PFCP) but sends
  no PDU Session Modification to the UE. Downlink enforcement happens in the UPF.
- **SBI OAuth2 is off** (`oauth: false` in `nrfcfg.yaml`), so the gateway needs no NRF token.
- **One UE, IPv4 only.** The device is identified by `device.ipv4Address`. With several UEs,
  upstream PCF 4.2.3 can bind an app session to the wrong UE.
- **Sessions live in memory.** Restarting the gateway forgets them.
- Bit rates show up in the UPF as Kbps, with 1 Mbps = 1024 Kbps.

## Run it

Tested on Ubuntu 24.04, kernel 6.8, Go 1.26.2, MongoDB 8.2, with passwordless `sudo`.

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

## The video

[video/](video/) builds the demo video. It records the steps above with VHS and tmux, adds the two
slides from [video/slides/](video/slides/) (an editable `qod-demo.pptx` is there too), and draws
highlights on the important lines. The narration lives in [video/narration/](video/narration/), and
[NOTES.md](NOTES.md) is the generated speaker script with timecodes.

```bash
cd video && ./setup.sh
.venv/bin/python build.py record    # runs the whole lab, about 3 minutes
.venv/bin/python build.py render    # out/qod-demo.mp4 (soft subtitles) and out/qod-demo-captions.mp4 (burned in)
```

To add your voice, record `video/recordings/<seg>-<NN>.wav` per cue (for example `d4-03.wav`) and
render again. Each cue is stretched to fit your take, and the captions follow it.
