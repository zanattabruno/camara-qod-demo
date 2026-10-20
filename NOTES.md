# Speaker notes: CAMARA QoD on free5GC

**Objective.** Show, end to end, that one standard API call really changes how the network treats traffic: an application asks for better QoS through the CAMARA Quality-on-Demand API, and an open-source 5G core (free5GC) enforces it. The demo follows that single request from the API, through 3GPP policy (gateway as AF → PCF → SMF), to the packet and QoS rules the UPF installs, and back out again when the session is deleted.

Video length: **7:08.5** (`video/out/qod-demo.mp4`). Each cue below has its start time, the words to say, what is typed on screen and what the orange highlight marks.

Two ways to add your voice:

1. **Per cue (recommended).** Record each cue as `video/recordings/<seg>-<NN>.wav` (for example `d4-03.wav`) and run `video/.venv/bin/python video/build.py render`. The video stretches each cue to your take, so pace is never a problem, and the captions follow your timing.
2. **One take over the video.** Play `video/out/qod-demo.mp4` with `video/out/qod-demo.srt` as a teleprompter and speak along. Cues are timed at 140 words per minute.

## s01 · Opening

**s01-01** · `0:00.5`  
Hello. My name is Gustavo Zanatta Bruno, and I'm a postdoc student of Professor Cristiano Bonato Both.  

**s01-02** · `0:08.5`  
The objective of this demo is to show, end to end, that one standard API call really changes how the network treats traffic. An application asks for better quality of service through the CAMARA Quality on Demand API, and we follow that request through the 3GPP policy functions down to the rules installed in the user plane, using only open source software.  

**s01-03** · `0:35.9`  
Everything runs on one Linux machine: free5GC release 4.2.3, UERANSIM to simulate the phone and the radio, and a small gateway that speaks CAMARA, version 1.1.0 of the API.  

## s02 · Architecture

**s02-01** · `0:49.5`  
On the left is the application. It knows nothing about 5G. It just calls the CAMARA API: give this device better QoS towards this server, with this profile, for this long.  
› highlight at `0:49.5`: slide area “app-call”  

**s02-02** · `1:03.6`  
Our gateway translates that into the 3GPP world. It acts as an application function and calls the PCF, the policy control function, on the N5 interface, using the Npcf Policy Authorization service.  
› highlight at `1:03.7`: slide area “gateway”  

**s02-03** · `1:18.0`  
The PCF turns the request into a policy rule and pushes it to the SMF. The SMF then programs the UPF, over PFCP, with new rules to detect our flow and enforce its bit rates.  
› highlight at `1:18.2`: slide area “core-path”  

**s02-04** · `1:33.8`  
This table is the whole translation. The device IP finds the PDU session, server and ports become a flow description, and the QoS profile becomes a media type with bit rates.  
› highlight at `1:33.9`: slide area “mapping”  

**s02-05** · `1:47.8`  
One note: in a production network this request goes through the NEF, the network exposure function. The free5GC NEF does not implement that QoS API, so in this lab the gateway talks to the PCF directly.  
› highlight at `1:48.0`: slide area “nef”  

## d1 · Run free5GC

**d1-01** · `2:04.5`  
Let's start the core. free5GC is cloned and built from the official repository, release 4.2.3. The install script in this repo does exactly that, with git clone and make.  
› types `git -C ~/free5gc describe --tags`  
› highlight at `2:06.0`: `^v4\.2\.3`  

**d1-02** · `2:17.6`  
The user plane needs gtp5g, the kernel module that carries the GTP tunnels. It is loaded.  
› types `lsmod | grep gtp5g`  
› highlight at `2:19.3`: `^gtp5g.*`  

**d1-03** · `2:25.2`  
One lab setting: OAuth is off on the service based interface, so our gateway can call the PCF without an NRF access token. A production core keeps it on.  
› types `grep -n oauth ~/free5gc/config/nrfcfg.yaml`  
› highlight at `2:27.4`: `oauth: false`  

**d1-04** · `2:38.4`  
Now we run free5GC with run dot s h. It starts the UPF, then each network function in turn: NRF, AMF, SMF, UDR, PCF, UDM, NSSF, AUSF, CHF and NEF.  
› types `sudo ./run.sh`  
› highlight at `2:40.1`: `NEF.*register to NRF successfully` in the core pane  

**d1-05** · `2:52.0`  
Each function registered itself in the NRF, which keeps the profiles in MongoDB. Here they are: the core is up.  
› types `mongosh --quiet free5gc --eval 'db.NfProfile.distinct("nfType")'`  
› highlight at `2:54.2`: `^\[ … \]$`  

## d2 · Subscriber, gNB and UE

**d2-01** · `3:01.8`  
Next, a subscriber. We start the webconsole, free5GC's management interface, and use its REST API to add our test SIM. It is the same call the Create subscriber form makes.  
› types `~/free5gc/webconsole/bin/webconsole -c ~/free5gc/config/webuicfg.yaml > /tmp/webconsole.log 2>&1 &`  
› types `./scripts/add-subscriber.sh`  

**d2-02** · `3:15.4`  
Created. The IMSI, the key and the OPc are now in the core's database.  
› highlight at `3:15.6`: `HTTP 201`  

**d2-03** · `3:22.2`  
Now the radio side. UERANSIM's gNB connects to the AMF over N2, and NG Setup succeeds.  
› types `~/UERANSIM/build/nr-gnb -c config/ueransim/gnb.yaml`  
› highlight at `3:24.4`: `NG Setup procedure is successful` in the gnb pane  

**d2-04** · `3:29.8`  
Then the UE. It registers, authenticates, and opens a PDU session on the internet data network. UERANSIM gives it a tunnel interface.  
› types `sudo ~/UERANSIM/build/nr-ue -c config/ueransim/ue.yaml`  
› highlight at `3:31.9`: `PDU Session establishment is successful … TUN interface` in the ue pane  

**d2-05** · `3:39.9`  
The device's IP address is 10.60.0.1. That is how the application will identify the device in its CAMARA request.  
› types `ip -br addr show uesimtun0`  
› highlight at `3:41.4`: `10\.60\.0\.1/\d+`  

## d3 · The CAMARA API and the translation

**d3-01** · `3:49.3`  
Now the API. The repo keeps the official CAMARA Quality on Demand definition, version 1.1.0, unmodified. Its operations: create, read and delete a session, extend it, and list a device's sessions.  
› types `grep -nE '^  /|^    (get|post|delete):' spec/quality-on-demand.yaml`  
› highlight at `3:51.6`: `/sessions:$ … /retrieve-sessions:`  

**d3-02** · `4:03.4`  
This is our request: the device by its IP address, the application server and the ports, the profile QOS_L, ten minutes, and a sink, the URL where we want status notifications.  
› types `batcat --style=numbers requests/create-session.json`  
› highlight at `4:05.6`: `"device" … "sink"`  

**d3-03** · `4:17.4`  
Profile names are defined by each operator. Our gateway maps them to 3GPP media components. QOS_L is video, which the PCF turns into 5QI 2, with up to 20 megabits downlink and 10 guaranteed.  
› types `batcat --style=numbers gateway/profiles.yaml`  
› highlight at `4:19.6`: `QOS_L.*`  

**d3-04** · `4:32.7`  
And this is the heart of the translation: the flow description that tells the network which packets get the better treatment, from the server to the device.  
› types `batcat --style=numbers -r 78:105 gateway/mapping.go`  
› highlight at `4:35.0`: `flow :=  … server, ports`  

## d4 · Create a QoD session

**d4-01** · `4:45.6`  
We start the gateway. It validates every request against the CAMARA definition, and sends policy requests to the PCF at 127.0.0.7.  
› types `cd gateway && go run . -insecure-sink`  

**d4-02** · `4:55.3`  
On the right, a tiny notification sink. It stands in for the application's webhook.  
› types `python3 scripts/sink.py`  

**d4-03** · `5:02.1`  
Now the request. HTTP 201: the session exists, its status is AVAILABLE, with a start and an expiry time.  
› types `./scripts/qod.sh create requests/create-session.json`  
› highlight at `5:04.4`: `"startedAt" … "qosStatus"`  

**d4-04** · `5:11.0`  
The gateway log shows the translation: media type video, the bit rates, the flow description, and the PCF answering 201 Created.  
› highlight at `5:11.1`: `POST /sessions … PCF 201 Created` in the gw pane  

**d4-05** · `5:20.7`  
And the application is notified with a CloudEvent: QoS status changed, AVAILABLE.  
› highlight at `5:20.8`: `qos-status-changed … "qosStatus": "AVAILABLE"` in the sink pane  

**d4-06** · `5:26.6`  
Inside the core: the PCF bound the request to the UE's PDU session and created an app session. The SMF installed a new PCC rule, and sent a PFCP modification that the UPF handled.  
› types `./scripts/corelog.sh`  
› highlight at `5:28.0`: `Session Binding Success … handleSessionModificationRequest`  

**d4-07** · `5:41.9`  
Here is the result in the data plane: two new packet detection rules that match exactly our flow, one per direction, ahead of the default rules.  
› types `./scripts/rules.sh`  
› highlight at `5:43.3`: `PDR 3  … PDR 4 `  

**d4-08** · `5:53.8`  
They point to a new QoS enforcement rule, QoS flow 2, capped at 20 megabits downlink with 10 guaranteed. free5GC counts 1024 kilobits per megabit.  
› highlight at `5:54.0`: `QER \d+  QFI 2 .*`  

## d5 · Read and delete

**d5-01** · `6:05.8`  
The application can poll the session at any time. It is still AVAILABLE.  
› types `./scripts/qod.sh get`  
› highlight at `6:06.7`: `"qosStatus": "AVAILABLE"`  

**d5-02** · `6:12.1`  
When it no longer needs the boost, it deletes the session. 204, no content.  
› types `./scripts/qod.sh delete`  
› highlight at `6:13.6`: `HTTP 204`  

**d5-03** · `6:18.9`  
The sink gets the last event: UNAVAILABLE, because the deletion was requested.  
› highlight at `6:19.0`: `"qosStatus": "UNAVAILABLE" … DELETE_REQUESTED` in the sink pane  

**d5-04** · `6:24.8`  
In the core, the PCF deleted the app session, and the SMF removed the PCC rule from the UPF.  
› types `./scripts/corelog.sh 4`  
› highlight at `6:26.2`: `Remove PCCRule.*`  

**d5-05** · `6:33.7`  
Only the default rules are left. The device is back to best effort.  
› types `./scripts/rules.sh`  
› highlight at `6:35.0`: `^PDRs … QER \d+  QFI 1 .*MBR DL 0 `  

**d5-06** · `6:40.0`  
That is the full cycle: one CAMARA API call, translated to N5, and enforced by a public 5G core. Two limits of this lab: the request skips the NEF, and the upstream SMF updates the UPF but does not signal the new QoS flow to the UE. The code and every step are in the repository. Thanks for watching.  
